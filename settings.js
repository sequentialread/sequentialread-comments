var _ = require('lodash');

var secrets = require('./secrets');

var settings = {
  recaptchaHost: "www.google.com",
  recaptchaPath: "/recaptcha/api/siteverify",
  origins: [
    "http://localhost:2368",
    "https://localhost:2368",
    "http://sequentialread.com",
    "https://sequentialread.com"
  ]
};

module.exports = _.merge(settings, secrets);
