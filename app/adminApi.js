

var fs = require('fs');
var Handlebars = require('handlebars');
var uuid = require('node-uuid');

var database = require('./database');
var settings = require('./settings');
var commentList = require('./commentList');
var template = Handlebars.compile(
  fs.readFileSync('./app/admin.html', 'utf8')
);
var landingPageTemplate = Handlebars.compile(
  fs.readFileSync('./app/adminLanding.html', 'utf8')
);

var hmacSha256 = require('../static/hmacSha256');

var authResult = {
  incorrect: null,
  missing: undefined
};

var nonce = uuid.v4();

module.exports = function (app) {

  app.get('/admin', function(req, res) {
    res.send(landingPageTemplate({nonce:nonce}));
  });

  app.post('/admin/comments', function(req, res) {
    var message = getAuthenticatedMessage(req.body);
    if(message) {
      commentsResponse(res);
    } else {
      loginResponse(res, message);
    }
  });

  app.post('/admin/delete', function(req, res) {
    var message = getAuthenticatedMessage(req.body);
    if(message) {
      database.deleteComment(
        message.delete.documentId,
        message.delete.date,
        function(err) {
          commentsResponse(res);
        }
      );
    } else {
      loginResponse(res, message);
    }
  });
};

function getAuthenticatedMessage(container) {

  if(!container.hmacSha256 || !container.message) {
    return authResult.missing;
  }

  // Verify that the nonces match and reset the nonce
  if(container.message.nonce != nonce) {
    return authResult.incorrect;
  }
  nonce = uuid.v4();

  var check = hmacSha256(JSON.stringify(container.message), settings.adminPassword);
  return check == container.hmacSha256 ? container.message : authResult.incorrect;
}

function commentsResponse(res) {
  database.getAllComments(function(err, comments) {
    res.send(template({
      nonce: nonce,
      authenticated: true,
      comments: commentList(comments),
      emptyMessage: "There are currently no comments."
    }));
  });
}

function loginResponse(res, message) {
  res.send(template({
    nonce: nonce,
    authenticated: false,
    incorrect: (message === authResult.incorrect)
  }));
}
