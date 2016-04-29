
var express = require('express');
var bodyParser = require('body-parser');
var url = require('url');

var settings = require('./settings');
var registerCommentsApi = require('./registerCommentsApi');

var app = express();
app.use( bodyParser.json() );

// CORS middleware, allow origins based on settings
app.use(function (req, res, next) {
  if(req.headers.referer) {
    var referer = url.parse(req.headers.referer);
    var origin = referer.protocol+(referer.slashes ? '//' : '')+referer.host;
    if(settings.origins.some(x => x === origin)) {
      res.setHeader('Access-Control-Allow-Origin', origin);
      res.setHeader('Access-Control-Allow-Methods', ['POST', 'GET', 'OPTIONS']);
      res.setHeader('Access-Control-Allow-Headers', ['Content-Type']);
    }
  }
  next();
});

// serve static files from the static folder
app.use('/static', express.static('static'));

// serve public comments api
registerCommentsApi(app);

var server = app.listen(settings.port, function () {
  var host = server.address().address;
  var port = server.address().port;

  console.log('Comments Api live at http://%s:%s', host, port);
});

module.exports = server;
