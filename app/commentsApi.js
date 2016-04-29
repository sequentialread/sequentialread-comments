
var _ = require('lodash');
var fs = require('fs');
var Handlebars = require('handlebars');
var md5 = require('md5');

var emailNotification = require('./emailNotification');
var database = require('./database');
var settings = require('./settings');
var validateGoogleCaptcha = require('./validateGoogleCaptcha');
var commentList = require('./commentList');

var templateHandlebars = Handlebars.compile(
  fs.readFileSync('./app/comments.html', 'utf8')
);
var template = function(data) {
  return templateHandlebars(_.merge(data, settings));
};

module.exports = function (app) {
  app.get('/api/*', function(req, res) {
    var documentId = req.params[0];

    commentsResponse(0, documentId, res);
  });

  app.post('/api/*', function(req, res) {
    var documentId = req.params[0];

    validateGoogleCaptcha(req.body['g-recaptcha-response'], function(err) {
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
}



function commentsResponse(error, documentId, res) {
  database.getCommentsForDocument(documentId, function(getCommentsForDocumentError, comments) {
    res.send(template({
      comments: commentList(comments),
      errors: [error, getCommentsForDocumentError],
      emptyMessage: "There are no comments on this post yet. "
          + "Your comment could be the first!"
    }));
  });
}

function postComment (documentId, comment, callback) {
  var email = comment.email ? comment.email.toLowerCase().trim() : null;
  if(email && email != '') {
    var hash = md5(email);
    comment.userId = hash.substring(5,10);
    comment.gravatarURL = comment.email && comment.email != '' ?
        'http://www.gravatar.com/avatar/' + hash
        : null;
  } else {
    comment.userId = '';
  }
  delete comment.email;

  comment.documentId = documentId;
  comment.date = Date.now();

  if(!comment.body || comment.body.trim() == "") {
    callback(new Error("comment body is required"));
  } else {
    database.saveComment(documentId, comment, function(err) {
      if(!err) {
        emailNotification("User " + comment.username + " commented \"" + comment.body + "\" on post #" + documentId);
      }
      callback(err);
    });
  }
}
