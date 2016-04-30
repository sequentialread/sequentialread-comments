
var levelup = require('levelup');
var fs = require('fs');

var commentsDir = './data/';
var commentsFileName = 'comments.db';

try {
  fs.mkdirSync(commentsDir);
} catch(ex) {
  if (ex.code != 'EEXIST') {
    throw e;
  }
}
var dbRaw = levelup(commentsDir+commentsFileName, { valueEncoding: 'json' });

module.exports = {
  saveComment: saveComment,
  deleteComment: deleteComment,
  getAllComments: getAllComments,
  getCommentsForDocument: getCommentsForDocument
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

function deleteComment (documentId, date, callback) {
  documentId = padDocumentId(documentId);
  date = addLeftZerosUntil(date, 15);
  console.log('del: '+documentId+'\x00'+date);
  dbRaw.del(documentId+'\x00'+date, function (err) {
    callback(err);
  });
}

function getAllComments (callback) {
  readComments({
    values    : true
  }, callback);
}

function getCommentsForDocument (documentId, callback) {
  documentId = padDocumentId(documentId);
  if(!documentId) {
    callback(new Error("invalid documentId"));
    return;
  }
  readComments({
    start     : documentId,
    end       : documentId+'\xff',
    values    : true
  }, callback);
}

function readComments(options, callback) {
  var buffer = [];
  dbRaw.createReadStream(options)
  .on('error', function (err) {
    console.error('getCommentsForDocument error ' + err.message);
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
