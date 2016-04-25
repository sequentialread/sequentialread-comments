
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
var marked = require('marked');
marked.setOptions({
  renderer: new marked.Renderer(),
  gfm: true,
  tables: true,
  breaks: false,
  pedantic: false,
  sanitize: true,
  smartLists: true,
  smartypants: false
});


var settings = require('./settings');
var emailNotification = require('./emailNotification');

var templateHandlebars = Handlebars.compile(fs.readFileSync('./template.html', 'utf8'));
var template = function(data) {
  return templateHandlebars(_.merge(data, settings));
};

var app = express();
app.use( bodyParser.json() );
var dbRaw = levelup('data/comments.db', { valueEncoding: 'json' });

var publishAtPort = process.env.PORT || 2369;

app.use(function (req, res, next) {
  if(req.headers.referer) {
    var referer = url.parse(req.headers.referer);
    var origin = referer.protocol+(referer.slashes ? '//' : '')+referer.host;
    if(settings.origins.some(x => x === origin)) {
      res.setHeader('Access-Control-Allow-Origin', origin);
      res.setHeader('Access-Control-Allow-Methods', ['POST', 'GET', 'OPTIONS']);
      res.setHeader('Access-Control-Allow-Headers', ['Content-Type']);
    }
  }
  next();
});

app.use(express.static('static'));

app.get('/api/*', function(req, res) {
  var documentId = padDocId(req.params[0], res);
  commentsResponse(0, documentId, res);
});

app.post('/api/*', function(req, res) {
  var documentId = padDocId(req.params[0], res);
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
    res.send(template({
      comments: comments.map(keyValue => {
        var comment = _.clone(keyValue.value);
        comment.body = marked(comment.body);
        return comment;
      }),
      errors: [error, getCommentsError]
    }));
  });
}

function postComment (documentId, post, callback) {
  if(!documentId) {
    callback(errorWithMessage("invalid documentId"));
    return;
  }

  var email = post.email ? post.email.toLowerCase().trim() : null;

  if(email && email != '') {
    var hash = md5(email);
    post.userId = hash.substring(5,10);
    post.gravatarURL = post.email && post.email != '' ?
        'http://www.gravatar.com/avatar/' + hash
        : null;
  } else {
    post.userId = '';
  }

  post.date = Date.now();

  if(!post.username || post.username.trim() == "") {
    post.username = "Unknown";
  }

  delete post.email;

  if(!post.body || post.body.trim() == "") {
    callback(errorWithMessage("post body is required"));
  } else {
    var datePartOfId = addLeftZerosUntil(post.date, 15);
    dbRaw.put(documentId+'\x00'+datePartOfId, post, function (err) {
      callback(err);
    });
    try {
      emailNotification("User " + post.username + " commented " + post.body + " on post #" + documentId);
    } catch (ex) {
      console.error(ex.message, ex.stack);
    }
  }
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
      } catch (ex) {
        console.error(ex.message, ex.stack);
      }
      callback(parsedData.success ? 0 : errorWithMessage('captcha validation failed'));
    });
  });

  req.write(postdata);

  req.end();
  req.on('error', (e) => {
    console.error(e);
  });
}

function padDocId (input, res) {
  if(input == null || input.length > 10 || !input.match(/[a-z0-9]*/i)) {
    return null;
  } else {
    return addLeftZerosUntil(input, 10);
  }
}

function addLeftZerosUntil(str, length) {
    str = String(str);
    while (str.length < length)
        str = "0" + str;
    return str;
}

function errorWithMessage(message) {
  var error = new Error();
  error.message = message;
  return error;
}

var server = app.listen(publishAtPort, function () {
  var host = server.address().address;
  var port = server.address().port;

  console.log('Comments Api live at http://%s:%s', host, port);
});
