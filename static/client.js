
(function(window, undefined){
  var commentContainer = document.getElementById('sqr-comment-container');
  var commentURL = commentContainer.getAttribute('data-comments-url');
  var commentForm;
  var submitButton;
  var captchaResult;

  function postComment() {
    var payload = Array.prototype.slice.call(commentForm)
      .reduce(function(result, x) {
        if(x.name && x.name != '') {
          result[x.name] = x.value;
        }
        return result;
      }, {});

    post(commentURL, payload, loadHTML);
  }

  function loadHTML(body) {
    commentContainer.innerHTML = body;
    commentForm = commentContainer.querySelector('.sqr-comment-form');
    submitButton = commentForm.querySelector('.submit');
    submitButton.onclick = function() {
      submitButton.disabled = true;
      submitButton.onclick = null;
      postComment();
    };

    var replyButton = commentContainer.querySelector('.btn.reply');
    replyButton.onclick = function() {
      commentForm.style.display = 'block';
      replyButton.style.display = 'none';
    };

    var datesSelection = commentContainer.querySelectorAll('.sqr-date');
    Array.prototype.slice.call(datesSelection).forEach(function(x){
      x.innerHTML = new Date(Number(x.innerHTML)).toDateString();
    });

    var captchaElement = commentContainer.querySelector('.g-recaptcha');
    var params = {
      sitekey: captchaElement.getAttribute('data-sitekey'),
      callback: window.recaptchaCompleted
    };
    window.grecaptcha.render(captchaElement, params);
  }

  window.recaptchaCompleted = function() {
    submitButton.disabled = false;
  }

  window.grecaptchaReady = function() {
    get(commentURL, loadHTML);
  }


  function get (url, callback) {
    xhr("GET", url, undefined, callback);
  }

  function post (url, body, callback) {
    xhr("POST", url, body, callback);
  }

  function xhr(method, url, body, callback) {
    var request = new XMLHttpRequest();
    request.addEventListener("load", function() {
      callback(this.responseText);
    });
    request.open(method, url);
    if(body && typeof body === 'object') {
      request.setRequestHeader("Content-Type", "application/json;charset=UTF-8");
      body = JSON.stringify(body);
    }
    request.send(body);
  }

})(window);
