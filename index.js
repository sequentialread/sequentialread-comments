
var express = require('express');
var bodyParser = require('body-parser');
var levelup = require('levelup');
var uuid = require('node-uuid');
var commentTemplateStreamFactory = require('./commentTemplateStreamFactory');
//var levelupIndexes = require('./levelup-indexes');
var app = express();
app.use( bodyParser.json() );
var dbRaw = levelup('data/comments2.db', { valueEncoding: 'json' });

var publishAtPort = process.env.PORT || 2369;

app.get('/comments/*', function(req, res) {
  getComments(req, res)
  .pipe(commentTemplateStreamFactory())
  .pipe(res);
});

app.post('/comments/*', function(req, res) {
  postComment(req, res);
});

// app.post('/reply/*/*', function(req, res) {
//   res.send('hello world');
// });

function postComment (req, res) {
  var documentId = validateDocId(req.params[0], res);
  dbRaw.put(documentId+'\x00'+uuid(), req.body, function (err) {
    if(err) {
      res.sendStatus(500);
    } else {
      res.sendStatus(200);
    }
  });
}

function getComments (req, res) {
  var documentId = validateDocId(req.params[0], res);
  return dbRaw.createReadStream({
    start     : documentId,
    end       : documentId+'\xff',
    values    : true
  }).on('error', function (err) {
    console.error('getComments error ' + err.message);
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
