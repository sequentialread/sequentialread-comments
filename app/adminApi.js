

var fs = require('fs');
var Handlebars = require('handlebars');

var database = require('./database');
var settings = require('./settings');
var commentList = require('./commentList');
var template = Handlebars.compile(fs.readFileSync('./app/admin.hbs', 'utf8'));
var landingPage = fs.readFileSync('./app/adminLanding.html', 'utf8');

var hmacSha256 = require('../static/hmacSha256');

var authResult = {
  incorrect: null,
  missing: undefined
};

module.exports = function (app) {

  app.get('/admin', function(req, res) {
    res.send(landingPage);
  });

  app.post('/admin/comments', function(req, res) {
    var message = getAuthenticatedMessage(req.body);
    if(message) {
      commentsResponse(res);
    } else {
      loginResponse(res);
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
      loginResponse(res);
    }
  });
};

function getAuthenticatedMessage(container) {
  if(!container.hmacSha256) {
    return authResult.missing;
  }
  var check = hmacSha256(JSON.stringify(container.message), settings.adminPassword);
  return check == container.hmacSha256 ? container.message : authResult.incorrect;
}

function commentsResponse(res) {
  database.getAllComments(function(err, comments) {
    res.send(template({
      authenticated: true,
      comments: commentList(comments),
      emptyMessage: "There are currently no comments."
    }));
  });
}

function loginResponse(res) {
  res.send(template({
    authenticated: false,
    incorrect: (message === authResult.incorrect)
  }));
}
