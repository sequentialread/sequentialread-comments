

module.exports = function (message) {
  var error = new Error();
  error.message = message;
  return error;
}
