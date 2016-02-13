var stream = require('stream');

module.exports = function (templateFunction) {
  var stringifier = new stream.Transform();
  stringifier._writableState.objectMode = true;
  stringifier._transform = function (data, encoding, done) {
      this.push(templateFunction(data));
      this.push('\n');
      done();
  }
  stringifier.on('error', function (err) {
    console.error('commentTemplate error ' + err.message + '\n' + err.stack);
  });
  return stringifier;
}
