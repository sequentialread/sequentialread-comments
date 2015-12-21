var stream = require('stream');

module.exports = function () {
  var stringifier = new stream.Transform();
  stringifier._writableState.objectMode = true;
  stringifier._transform = function (data, encoding, done) {
      this.push(JSON.stringify(data));
      this.push('\n');
      done();
  }
  stringifier.on('error', function (err) {
    console.error('commentTemplate error ' + err.message + '\n' + err.stack);
  });
  return stringifier;
}
