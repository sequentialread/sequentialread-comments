
var fs = require('fs');
var rawSettings = JSON.parse(fs.readFileSync("./settings.json", "utf8"));
var filteredSettings = {};

//$GMAIL_PASSWORD
var unixEnvironmentVariableRegex = /^\$[A-Z_]+$/;

for(var name in rawSettings) {
  if(rawSettings.hasOwnProperty(name)) {
    var rawValue = rawSettings[name];
    if(unixEnvironmentVariableRegex.test(rawValue)) {
      filteredSettings[name] = process.env[rawValue.replace("$","")];
    } else {
      filteredSettings[name] = rawValue;
    }
  }
}

module.exports = filteredSettings;
