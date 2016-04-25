
module.exports = {
  recaptchaHost: "www.google.com",
  recaptchaPath: "/recaptcha/api/siteverify",
  recaptchaSiteKey: "6Le7MB4TAAAAAIIky6MbeT---DSCSXic3pQSTPLh",
  recaptchaSecretKey: process.env.RECAPTCHA_SECRET_KEY,
  origins: [
    "http://localhost:2368",
    "https://localhost:2368",
    "http://sequentialread.com",
    "https://sequentialread.com"
  ]
};
