
var express = require('express');
var bodyParser = require('body-parser');
var levelup = require('levelup');
var uuid = require('node-uuid');
//var templateStreamFactory = require('./templateStreamFactory');
var fs = require('fs');
var template = Handlebars.compile(fs.readFileSync('./template.html', 'utf8'));
//var levelupIndexes = require('./levelup-indexes');
var app = express();
app.use( bodyParser.json() );
var dbRaw = levelup('data/comments3.db', { valueEncoding: 'json' });

var publishAtPort = process.env.PORT || 2369;

app.get('/comments/*', function(req, res) {
  var documentId = validateDocId(req.params[0], res);
  getComments(documentId, function(err, comments) {
    if(err) {
      res.sendStatus(500);
    }
    res.send(template({comments: comments}));
  });
});

app.post('/comments/*', function(req, res) {
  var documentId = validateDocId(req.params[0], res);
  postComment(documentId, req.body, function(err) {
    res.sendStatus(err ? 500 : 200);
  });
});

// app.post('/reply/*/*', function(req, res) {
//   res.send('hello world');
// });

function postComment (documentId, body, callback) {
  dbRaw.put(documentId+'\x00'+uuid(), body, function (err) {
    callback(err);
  });
}

function getComments (documentId, callback) {
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
    callback(buffer);
  });
}


var server = app.listen(publishAtPort, function () {
  var host = server.address().address;
  var port = server.address().port;

  console.log('Example app listening at http://%s:%s', host, port);
});


function validateDocId (input, res) {
  if(input == null || input.length != 40 || !input.match(/[a-z0-9]*/i)) {
    res.sendStatus(500);
  } else {
    return input;
  }
}
