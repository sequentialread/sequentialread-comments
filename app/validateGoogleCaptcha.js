
var https = require('https');
var url = require('url');
var querystring = require('querystring');

var settings = require('./settings');

var recaptchaHost = "www.google.com";
var recaptchaPath = "/recaptcha/api/siteverify";

module.exports = function validateGoogleCaptcha(captchaResponse, callback) {

  var postdata = querystring.stringify({
      'secret' : settings.recaptchaSecretKey,
      'response': captchaResponse
      // this is optional per google's API
      //'remoteip': request.connection.remoteAddress
  });

  var options = {
    hostname: recaptchaHost,
    path: recaptchaPath,
    port: 443,
    method: 'POST',
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
      'Content-Length': Buffer.byteLength(postdata)
    }
  };

  var req = https.request(options, (res) => {
    var data = "";
    res.on('data', chunk => data += chunk.toString());
    res.on('end', function() {
      var parsedData = { success: false };
      try {
        parsedData = JSON.parse(data);
      } catch (ex) {
        console.error(ex.message, ex.stack);
      }
      callback(parsedData.success ? 0 : new Error('captcha validation failed'));
    });
  });

  req.write(postdata);

  req.end();
  req.on('error', (e) => {
    console.error(e);
  });
}
