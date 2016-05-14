
var express = require('express');
var bodyParser = require('body-parser');
var url = require('url');

var settings = require('./settings');
var registerAdminApi = require('./adminApi');
var registerCommentsApi = require('./commentsApi');

var app = express();
app.use( bodyParser.json() );

// CORS middleware, allow origins based on settings
app.use(function (req, res, next) {
  if(req.headers.origin) {
    var originUrl = url.parse(req.headers.origin);
    var origin = originUrl.protocol+(originUrl.slashes ? '//' : '')+originUrl.host;

    if(settings.origins.some(x => x === origin)) {
      res.setHeader('Access-Control-Allow-Origin', origin);

      if(req.headers['access-control-request-method']) {
        res.setHeader('Access-Control-Allow-Methods', req.headers['access-control-request-method']);
      }
      if(req.headers['access-control-request-headers']) {
        res.setHeader('Access-Control-Allow-Headers', req.headers['access-control-request-headers']);
      }
    }
  }
  next();
});

// serve static files from the static folder
app.use('/static', express.static('static'));

// serve public comments api
registerCommentsApi(app);

// serve admin page
registerAdminApi(app);

var server = app.listen(settings.port, function () {
  var host = server.address().address;
  var port = server.address().port;

  console.log('Comments Api live at http://%s:%s', host, port);
});

module.exports = server;
