
var express = require('express');
var bodyParser = require('body-parser');
var levelup = require('levelup');
var uuid = require('node-uuid');
var fs = require('fs');
var https = require('https');
var url = require('url');
var querystring = require('querystring');
var Handlebars = require('handlebars');
var _ = require('lodash');
var md5 = require('md5');

var settings = require('./settings');

var templateHandlebars = Handlebars.compile(fs.readFileSync('./template.html', 'utf8'));
var template = function(data) {
  return templateHandlebars(_.merge(settings, data));
};

var app = express();
app.use( bodyParser.json() );
var dbRaw = levelup('data/comments7.db', { valueEncoding: 'json' });

var publishAtPort = process.env.PORT || 2369;

app.use(function (req, res, next) {
  var referer = url.parse(req.headers.referer);
  var origin = referer.protocol+(referer.slashes ? '//' : '')+referer.host;
  if(settings.origins.some(x => x === origin)) {
    res.setHeader('Access-Control-Allow-Origin', origin);
    res.setHeader('Access-Control-Allow-Methods', ['POST', 'GET', 'OPTIONS']);
    res.setHeader('Access-Control-Allow-Headers', ['Content-Type']);
  }
  next();
});

app.use(express.static('static'));

app.get('/comments/*', function(req, res) {
  var documentId = validateDocId(req.params[0], res);
  commentsResponse(0, documentId, res);
});

app.post('/comments/*', function(req, res) {
  var documentId = validateDocId(req.params[0], res);
  validateCaptcha(req.body['g-recaptcha-response'], function(err) {
    if(!err) {
      delete req.body['g-recaptcha-response'];

      postComment(documentId, req.body, function(err) {
        commentsResponse(err, documentId, res);
      });
    } else {
      commentsResponse(err, documentId, res);
    }
  });
});

function commentsResponse(error, documentId, res) {
  getComments(documentId, function(getCommentsError, comments) {
    console.log({comments: comments, errors: [error, getCommentsError]});
    res.send(template({
      comments: comments.map(x => x.value),
      errors: [error, getCommentsError]
    }));
  });
}

function postComment (documentId, body, callback) {
  if(!documentId) {
    callback(errorWithMessage("invalid documentId"));
    return;
  }

  var hash = md5(body.email.toLowerCase().trim());
  console.log(hash);

  body.userId = hash.substring(5,5);

  body.gravatarURL = body.email && body.email != '' ?
      'http://www.gravatar.com/avatar/' + hash
      : null;

  body.date = Date.now();

  delete body.email;

  dbRaw.put(documentId+'\x00'+uuid(), body, function (err) {
    callback(err);
  });
}

function getComments (documentId, callback) {
  if(!documentId) {
    callback(errorWithMessage("invalid documentId"));
    return;
  }
  var buffer = [];
  dbRaw.createReadStream({
    start     : documentId,
    end       : documentId+'\xff',
    values    : true
  }).on('error', function (err) {
    console.error('getComments error ' + err.message);
    callback(err);
  }).on('data', function(data) {
    buffer.push(data);
  }).on('close', function() {
    callback(0, buffer);
  });
}

function validateCaptcha(captchaResponse, callback) {

  var postdata = querystring.stringify({
      'secret' : settings.recaptchaSecretKey,
      'response': captchaResponse
      //'remoteip': request.connection.remoteAddress
  });

  console.log(postdata);

  var options = {
    hostname: settings.recaptchaHost,
    path: settings.recaptchaPath,
    port: 443,
    method: 'POST',
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
      'Content-Length': Buffer.byteLength(postdata)
    }
  };

  var req = https.request(options, (res) => {
    var data = "";
    res.on('data', chunk => data += chunk.toString());
    res.on('end', function() {
      var parsedData = { success: false };
      try {
        parsedData = JSON.parse(data);
      } catch (ex) {}
      callback(parsedData.success ? 0 : errorWithMessage('captcha validation failed'));
    });
  });

  req.write(postdata);

  req.end();
  req.on('error', (e) => {
    console.error(e);
  });
}

function validateDocId (input, res) {
  if(input == null || input.length > 10 || !input.match(/[a-z0-9]*/i)) {
    return null;
  } else {
    return input;
  }
}

function errorWithMessage(message) {
  var error = new Error();
  error.message = message;
  return error;
}

var server = app.listen(publishAtPort, function () {
  var host = server.address().address;
  var port = server.address().port;

  console.log('Example app listening at http://%s:%s', host, port);
});
