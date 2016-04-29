(function(global, module, sha256, undefined){
	var hmacConstant_i = "\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\";
	var hmacConstant_o = "6666666666666666";

	function xorString(a, b) {
		var result = '';
		var aIsLonger = a.length > b.length;
		var longer = aIsLonger ? a : b;
		var shorter = aIsLonger ? b : a;
		for(i=0; i < longer.length; i++) {
			var longerCharCode = longer[i].charCodeAt(0);
			var shorterCharCode = i < shorter.length ? shorter[i].charCodeAt(0) : 0;
			result += String.fromCharCode(longerCharCode ^ shorterCharCode);
		}
		return result;
	}

	function hmacSha256(message, pw) {
		var padded_i = xorString(hmacConstant_i, pw);
		var padded_o = xorString(hmacConstant_o, pw);
		var messageDigest = sha256(padded_i + message);
		var hmac = sha256(padded_o + messageDigest);
		return hmac;
	}

  if(module) {
    module.exports = hmacSha256;
  } else {
    global.hmacSha256 = hmacSha256;
  }
})(
  this,
  typeof module !== 'undefined' ? module : null,
  typeof module !== 'undefined' ? require('./sha256') : this.sha256
);
