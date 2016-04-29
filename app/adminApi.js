

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
    var message = getAuthenticatedMessage(req);
    if(message) {
      commentsResponse(res);
    } else {
      loginResponse(res, message);
    }
  });

  app.post('/admin/delete', function(req, res) {
    var message = getAuthenticatedMessage(req);
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

function getAuthenticatedMessage(req) {
  if(!req.body || !req.headers.authorization) {
    return authResult.missing;
  }

  var check = hmacSha256(JSON.stringify(req.body)+nonce, settings.adminPassword);

  nonce = uuid.v4();

  return check == req.headers.authorization ? req.body : authResult.incorrect;
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
