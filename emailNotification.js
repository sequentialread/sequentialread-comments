var settings = require('./settings');
var nodemailer = require('nodemailer');

var gmailSmtpConfig = {
    host: settings.emailHost,
    port: settings.emailPort,
    secure: true, // use SSL
    auth: {
        user: settings.emailUsername,
        pass: settings.emailPassword
    }
};

var gmailTransport = nodemailer.createTransport(gmailSmtpConfig);

module.exports = function(notification) {
  var mail = {
    from: settings.emailUsername,
    to: settings.emailNotificationTarget,
    subject: 'New Comment Notification',
    text: notification
  };
  gmailTransport.sendMail(mail, function(error, info){
    if(error){
        return console.log(error);
    }
    console.log('Message sent: ' + info.response);
  });
}
