
(function(window, document, undefined){
  const commentContainer = document.getElementById('sqr-comment-container');
  let commentsURL = commentContainer.getAttribute('data-comments-url');
  if(commentsURL.endsWith("/")) {
    commentsURL = commentsURL.substring(0, commentsURL.length-1)
  }
  const documentID = commentContainer.getAttribute('data-comments-document-id');
  let commentForm;
  let submitButton;

  xhr("GET", `${commentsURL}/api/${documentID}?t=${Date.now()}`, undefined, displayCommentsFromJSON);

  function displayCommentsFromJSON(responseRaw, autoExpand) {
    try {
      let response;
      if(typeof responseRaw == "string") {
        response = JSON.parse(responseRaw);
      } else {
        response = responseRaw;
      }
      if(response.captchaURL.endsWith("/")) {
        response.captchaURL = response.captchaURL.substring(0, response.captchaURL.length-1);
      }
      
      if(!document.querySelector(`link[href='${commentsURL}/static/comments.css']`)) {
        createElement(document.head, "link", {
          "rel": "stylesheet",
          "charset": "utf8",
          "href": `${commentsURL}/static/comments.css`,
        });
      }
      if(!document.querySelector(`link[href='${response.captchaURL}/static/captcha.css']`)) {
        createElement(document.head, "link", {
          "rel": "stylesheet",
          "charset": "utf8",
          "href": `${response.captchaURL}/static/captcha.css`,
        });
      }

      commentContainer.innerHTML = "";

      const replyButton = createElement(commentContainer, "button", { "class": "sqr-btn sqr-reply" });
      createElement(replyButton, "i", { "class": "fa fa-reply" }, "");
      appendFragment(replyButton, " reply ");

      commentForm = createElement(commentContainer, "form", { 
        "class": "sqr-comment-form",
        "method": "POST",
        "action": "#",
      });

      replyButton.onclick = function() {
        commentForm.style.display = 'block';
        replyButton.style.display = 'none';
      };

      if(autoExpand) {
        replyButton.onclick();
      }

      const nameLabel = createElement(commentForm, "label", null, "Name: ");
      createElement(nameLabel, "input", { "name": "username" });
      createElement(commentForm, "br");
      const emailLabel = createElement(commentForm, "label", null, "Email: ");
      createElement(emailLabel, "input", { "name": "email" });
      createElement(
        emailLabel, 
        "span", 
        { "class": "sqr-help-text" }, 
        "  (optional, not stored, used for Gravatar)"
      );
      createElement(commentForm, "textarea", { "name": "body", "rows": "5" });
      createElement(commentForm, "p", null, "Markdown is supported.");
      const submitArea = createElement(commentForm, "div", { "class": "sqr-submit-area" });
      const nonceInput = createElement(submitArea, "input", { "name": "captchaNonce", "type": "hidden" });
      const challengeInput = createElement(submitArea, "input", { "name": "captchaChallenge", "type": "hidden" });
      challengeInput.value = response.captchaChallenge;

      const captchaElement = createElement(submitArea, "div");
      captchaElement.dataset.sqrCaptchaUrl = response.captchaURL;
      captchaElement.dataset.sqrCaptchaChallenge = response.captchaChallenge;
      captchaElement.dataset.sqrCaptchaCallback = "sqrCaptchaCompleted";
      window.sqrCaptchaCompleted = (nonce) => {
        nonceInput.value = nonce;
        submitButton.disabled = false;
      };

      if(!document.querySelector(`script[src='${response.captchaURL}/static/captcha.js']`)) {
        createElement(document.head, "script", {
          "type": "text/javascript",
          "src": `${response.captchaURL}/static/captcha.js`,
        });
      } else if(window.sqrCaptchaInit) {
        if(autoExpand) {
          window.sqrCaptchaReset();
        }
        window.sqrCaptchaInit();
      } else {
        console.log("captcha.js was already loaded, but sqrCaptchaInit was not found. Continuing...");
      }
      submitButton = createElement(submitArea, "button", { 
        "class": "sqr-btn sqr-submit",
        "disabled": true
      }, "submit");

      submitButton.onclick = function(event) {
        submitButton.disabled = true;
        submitButton.onclick = null;
        postComment();
  
        // dont leave the page and post the form since we are doing xhrs
        event.preventDefault();
        return false;
      };

      if(response.error) {
        createElement(commentContainer, "div", { "class": "sqr-error" }, `Error: ${response.error}`);
      }

      if(!response.comments || response.comments.length == 0) {
        createElement(
          commentContainer, 
          "div", 
          null, 
          "There are no comments on this post yet. Your comment could be the first!"
        );
        return
      }
      const comments = createElement(commentContainer, "div", { "class": "sqr-comments" });
      response.comments.forEach(x => {
        const comment = createElement(comments, "div", { "class": "sqr-comment" });

        createElement(comment, "img", { 
          "class": "sqr-gravatar",
          "src": `https://www.gravatar.com/avatar/${x.gravatarHash}?d=retro`
        });
        const postColumn = createElement(comment, "div", { "class": "sqr-post-column" });
        const postRow = createElement(postColumn, "div");
        createElement(postRow, "span", { "class": "sqr-username" }, x.username);
        createElement(postRow, "span", { "class": "sqr-userid" }, x.userId);
        //createElement(postRow, "span", { "class": "sqr-documentId" }, x.documentId);
        createElement(
          postRow, 
          "span", 
          { "class": "sqr-date" }, 
          new Date(Number(x.date)).toDateString()
        );
        const content = createElement(postColumn, "p");
        // TODO migrate to DOMPurify for this ?
        content.innerHTML = x.bodyHTML;
      });

    } catch (err) {
      console.error(err)
      console.log(`stack: ${err.stack}`)
      commentContainer.textContent = "error loading comments 😓"
    }
  };

  function postComment() {
    var payload = Array.prototype.slice.call(commentForm)
      .reduce(function(result, x) {
        if(x.name && x.name != '') {
          result[x.name] = x.value;
        }
        return result;
      }, {});

    xhr("POST", `${commentsURL}/api/${documentID}`, payload, response => displayCommentsFromJSON(response, true));
  }

  window.sqrCaptchaCompleted = function() {
    submitButton.disabled = false;
  }

  function createElement(parent, tag, attr, textContent) {
    const element = document.createElement(tag);
    if(attr) {
      Object.entries(attr).forEach(kv => element.setAttribute(kv[0], kv[1]));
    }
    if(textContent) {
      element.textContent = textContent;
    }
    parent.appendChild(element);
    return element;
  }

  function appendFragment(parent, textContent) {
    const fragment = document.createDocumentFragment()
    fragment.textContent = textContent
    parent.appendChild(fragment)
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

})(window, document);
