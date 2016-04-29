
var levelup = require('levelup');
var fs = require('fs');

try {
  fs.mkdirSync('./data');
} catch(ex) {
  if (ex.code != 'EEXIST') {
    throw e;
  }
}
var dbRaw = levelup('./data/comments.db', { valueEncoding: 'json' });

module.exports = {
  saveComment: saveComment,
  getComments: getComments
};

function saveComment (documentId, comment, callback) {
  documentId = padDocumentId(documentId);
  if(!documentId) {
    callback(new Error("invalid documentId"));
    return;
  }

  var datePartOfId = addLeftZerosUntil(comment.date, 15);
  dbRaw.put(documentId+'\x00'+datePartOfId, comment, function (err) {
    callback(err);
  });
}

function getComments (documentId, callback) {
  documentId = padDocumentId(documentId);
  if(!documentId) {
    callback(new Error("invalid documentId"));
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

function padDocumentId (input, res) {
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
