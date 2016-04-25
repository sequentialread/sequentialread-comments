
module.exports = {
  recaptchaHost: "www.google.com",
  recaptchaPath: "/recaptcha/api/siteverify",
  recaptchaSiteKey: "6Le7MB4TAAAAAIIky6MbeT---DSCSXic3pQSTPLh",
  recaptchaSecretKey: process.env.RECAPTCHA_SECRET_KEY,
  origins: [
    "http://localhost:2368",
    "https://localhost:2368",
    "http://sequentialread.com",
    "https://sequentialread.com",
    "http://blog.sequentialread.com",
    "https://blog.sequentialread.com"
  ],
  emailHost: 'smtp.gmail.com',
  emailPort: 465,
  emailUsername: process.env.GMAIL_USER,
  emailPassword: process.env.GMAIL_PASSWORD,
  emailNotificationTarget: process.env.EMAIL_NOTIFICATION_TARGET
};
