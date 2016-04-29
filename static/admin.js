
(function(window, undefined){

  var container = document.getElementById('sqr-admin-container');

  var sessionStorageName = 'sqr-admin-password';
  var adminPassword =  window.sessionStorage ? window.sessionStorage[sessionStorageName] : null;
  var query = {skip: 0};

  postWithHmac("/comments", query, loadHTML);

  function loadHTML(body) {
    container.style.display = 'block';
    container.innerHTML = body;

    var datesSelection = container.querySelectorAll('.sqr-date');
    Array.prototype.slice.call(datesSelection).forEach(function(x) {
      x.parentElement.appendChild(makeDeleteButton(x));
      x.innerHTML = new Date(Number(x.innerHTML)).toDateString();
    });

    var loginForm = container.querySelector('form.login');
    if(loginForm) {
      loginForm.onsubmit = doLogin;
    }

    delete query.delete;
  }

  function doLogin() {
    adminPassword = container.querySelector('input[type="password"]').value;
    if(window.sessionStorage) {
      window.sessionStorage[sessionStorageName] = adminPassword;
    }
    postWithHmac("/comments", query, loadHTML);

    // prevent default html form behaviour
    return false;
  };

  function makeDeleteButton(dateElement) {
    var deleteButton = document.createElement('button');
    var documentId = dateElement.parentElement
      .querySelector('.sqr-documentId').innerHTML;
    var date = dateElement.innerHTML;

    deleteButton.className = "sqr-btn-delete";
    deleteButton.innerHTML = "delete";
    deleteButton.onclick = function() {
      query.delete = {
        documentId: documentId,
        date: date
      };
      postWithHmac("/delete", query, loadHTML);
    }
    return deleteButton;
  }

  function postWithHmac (endpoint, body, callback) {
    var signedBody = {
      message: body
    };
    if(adminPassword && adminPassword != '') {
      body.nonce = container.querySelector('input[type="hidden"]').value;
      signedBody.hmacSha256 = window.hmacSha256(JSON.stringify(body), adminPassword);
    }
    xhr("POST", window.location.href+endpoint, signedBody, callback);
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
