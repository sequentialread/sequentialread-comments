
var express = require('express');
var bodyParser = require('body-parser');
var levelup = require('levelup');
var uuid = require('node-uuid');
var fs = require('fs');
var https = require('https');
var querystring = require('querystring');
var Handlebars = require('handlebars');

var settings = require('./secrets');

var template = Handlebars.compile(fs.readFileSync('./template.html', 'utf8'), settings);

var app = express();
app.use( bodyParser.urlencoded() );
var dbRaw = levelup('data/comments3.db', { valueEncoding: 'json' });

var publishAtPort = process.env.PORT || 2369;

app.get('/comments/*', function(req, res) {


  var documentId = validateDocId(req.params[0], res);
  getComments(documentId, function(err, comments) {
    console.log(err);
    console.log(comments);
    if(err) {
      res.send(err);
    } else {
      res.send(template({comments: comments}));
    }
  });
});

app.post('/comments/*', function(req, res) {
  var documentId = validateDocId(req.params[0], res);
  console.log(req);
  postComment(documentId, req.body, function(err) {
    res.sendStatus(err ? 500 : 200);
  });
});

// app.post('/reply/*/*', function(req, res) {
//   res.send('hello world');
// });

function postComment (documentId, body, callback) {
  if(!documentId) {
    callback(new Error("no documentId provided"));
    return;
  }
  console.log(body);
  dbRaw.put(documentId+'\x00'+uuid(), body, function (err) {
    callback(err);
  });
}

function getComments (documentId, callback) {
  if(!documentId) {
    console.log("no documentId provided");
    callback(new Error("no documentId provided"));
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


var server = app.listen(publishAtPort, function () {
  var host = server.address().address;
  var port = server.address().port;

  console.log('Example app listening at http://%s:%s', host, port);
});

function validateRecaptcha(clientRequest, callback) {
  var options = {
    hostname: settings.recaptchaHost,
    path: settings.recaptchaPath,
    port: 443,
    method: 'POST'
  };
  var req = https.request(options, (res) => {
    console.log('statusCode: ', res.statusCode);
    console.log('headers: ', res.headers);

    res.on('data', (d) => {
      process.stdout.write(d);
    });
  });
  req.write(querystring.stringify({
      'secret' : settings.recaptchaSecretKey,
      'response': clientRequest['g-recaptcha-response'],
      'remoteip': clientRequest.connection.remoteAddress
  }));
  req.end();
  req.on('error', (e) => {
    console.error(e);
  });
}

function validateDocId (input, res) {
  if(input == null || input.length != 32 || !input.match(/[a-z0-9]*/i)) {
    return null;
  } else {
    return input;
  }
}
