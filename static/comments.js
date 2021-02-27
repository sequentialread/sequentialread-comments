
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

  let currentFormContainer;

  function displayCommentsFromJSON(responseRaw, justPostedReplyTo) {
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

      const rootReplyButton = createElement(commentContainer, "button", { "class": "sqr-btn sqr-reply" });
      createElement(rootReplyButton, "i", { "class": "fa fa-reply" }, "");
      appendFragment(rootReplyButton, " leave a comment ");

      const rootFormContainer = createElement(commentContainer, "div");

      rootReplyButton.onclick = function() {
        rootReplyButton.style.display = 'none';
        displayCommentForm(rootFormContainer, response, "root");
      };

      if(justPostedReplyTo == "root" && response.error) {
        rootReplyButton.onclick();
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
      const indentEmPerReply = 2;
      let mostRecentComment = "";
      const displayComment = (parent, parentComment, x, indent) => {
        const postID = `${x.documentId}_${x.date}`;
        if(!mostRecentComment || mostRecentComment < postID) {
          mostRecentComment = postID
        }
        const comment = createElement(comments, "div", { "class": "sqr-comment", "id": postID });
        comment.style.marginLeft = `${indent}em`

        createElement(comment, "img", { 
          "class": "sqr-gravatar",
          "src": `${commentsURL}/avatar/${x.avatarHash}`
        });
        const postColumn = createElement(comment, "div", { "class": "sqr-post-column" });
        if(window.location.hash == `#${postID}`) {
          postColumn.classList.add("sqr-highlighted");
          postColumn.scrollIntoView();
        }
        const postRow = createElement(postColumn, "div");
        createElement(postRow, "span", { "class": "sqr-username" }, x.username);
        createElement(postRow, "span", { "class": "sqr-avatar-hash" }, x.avatarHash);
        if(parentComment) {
          const inReplyTo = createElement(postRow, "span", { "class": "sqr-in-reply-to" }, " in reply to ");
          inReplyTo.innerHTML = `${inReplyTo.innerHTML}&nbsp;&nbsp;&nbsp;`;
          createElement(inReplyTo, "span", { "class": "sqr-username" }, parentComment.username);
          createElement(inReplyTo, "span", { "class": "sqr-avatar-hash" }, parentComment.avatarHash);
        }
        //createElement(postRow, "span", { "class": "sqr-documentId" }, x.documentId);
        createElement(
          postRow, 
          "span", 
          { "class": "sqr-date" }, 
          new Date(Number(x.date)).toDateString()
        );
        const content = createElement(postColumn, "div");
        const bottomRow = createElement(postColumn, "div", {"class": "sqr-comment-bottom-row"});
        const linkURL = `${window.location.href.split("#")[0]}#${postID}`;
        const linkElement = createElement(bottomRow, "a", {"href": linkURL}, "🔗 link");
        linkElement.onclick = () => {
          Array.from(document.querySelectorAll(".sqr-post-column")).forEach(x => x.classList.remove("sqr-highlighted"));
          postColumn.classList.add("sqr-highlighted");
        };
        appendFragment(bottomRow, " | ")
        const replyButton = createElement(bottomRow, "span", {}, "💬 reply");
        const formContainer = createElement(comment, "div", {
          "id": `sqr-form-container-${postID}`,
          "class": "sqr-reply-to-comment-form"
        });
        replyButton.onclick = () => {
          rootReplyButton.style.display = 'inline-block';
          displayCommentForm(formContainer, response, postID)
        };
        if(justPostedReplyTo == postID && response.error) {
          replyButton.onclick();
        }
        // TODO migrate to DOMPurify for this ?
        content.innerHTML = x.bodyHTML;

        if(x.replies) {
          x.replies.forEach(y => displayComment(comments, x, y, indent+indentEmPerReply))
        }
      };

      // display the comment tree
      response.comments.forEach(x =>  displayComment(comments, null, x, 0));

      if(justPostedReplyTo && !response.error) {
        const justPostedElement = document.getElementById(mostRecentComment);
        if(justPostedElement) {
          justPostedElement.querySelector(".sqr-post-column").classList.add("sqr-highlighted");
          justPostedElement.scrollIntoView();
        }
      }

    } catch (err) {
      console.error(err)
      console.log(`stack: ${err.stack}`)
      commentContainer.textContent = "error loading comments 😓"
    }
  };

  function displayCommentForm(parent, response, inReplyTo) {

    if (currentFormContainer) {
      currentFormContainer.innerHTML = "";
      currentFormContainer.style.display = "none";
    }
    currentFormContainer = parent;
    currentFormContainer.style.display = "block";

    commentForm = createElement(parent, "form", { 
      "class": "sqr-comment-form",
      "method": "POST",
      "action": "#",
    });
    
    const nameLabel = createElement(commentForm, "label", null, "Name: ");
    createElement(nameLabel, "input", { "type": "text", "name": "username" });
    createElement(commentForm, "br");
    const emailLabel = createElement(commentForm, "label", null, "Email: ");
    createElement(emailLabel, "input", { "type": "text", "name": "email" });
    appendFragment(emailLabel, " (optional)");

    createElement(commentForm, "br");
    const notifyRow = createElement(commentForm, "div", { "class": "sqr-radio-row" }, "Email Notifications: ");

    const notifyOffLabel = createElement(notifyRow, "label" , { "class": "sqr-selected-radio" });
    createElement(notifyOffLabel, "input", { type: "radio", name: "notifyOfReplies", value: "off", checked: "checked"});
    createElement(notifyOffLabel, "span", null, " Off");

    const notifyThreadLabel = createElement(notifyRow, "label");
    createElement(notifyThreadLabel, "input", { type: "radio", name: "notifyOfReplies", value: "child+sibling" });
    createElement(notifyThreadLabel, "span", null,  " Notify on New Replies");

    Array.from(notifyRow.querySelectorAll("input")).forEach(x => x.addEventListener("change", () => {
      Array.from(notifyRow.querySelectorAll("input")).forEach(y => {
        y.parentElement.classList[y.checked ? 'add' : 'remove']("sqr-selected-radio");
      });
    }));

    const avatarRow = createElement(commentForm, "div", { "class": "sqr-radio-row" }, "Avatar Image: ");

    const identiconLabel = createElement(avatarRow, "label" , { "class": "sqr-selected-radio" });
    createElement(identiconLabel, "input", { type: "radio", name: "avatarType", value: "sha256identicon", checked: "checked"});
    createElement(identiconLabel, "span", null,  " Generate");

    const gravatarLabel = createElement(avatarRow, "label");
    createElement(gravatarLabel, "input", { type: "radio", name: "avatarType", value: "gravatar" });
    createElement(gravatarLabel, "span", null,  " Gravatar");

    Array.from(avatarRow.querySelectorAll("input")).forEach(x => x.addEventListener("change", () => {
      Array.from(avatarRow.querySelectorAll("input")).forEach(y => {
        y.parentElement.classList[y.checked ? 'add' : 'remove']("sqr-selected-radio");
      });
    }));

    createElement(commentForm, "input", { "type": "hidden", "name": "inReplyTo", "value": inReplyTo });
    createElement(commentForm, "input", { "type": "hidden", "name": "url", "value": window.location.href });
    createElement(commentForm, "input", { "type": "hidden", "name": "documentTitle", "value": document.title });

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
      window.sqrCaptchaReset();
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
      createElement(parent, "div", { "class": "sqr-error" }, `Error: ${response.error}`);
    }
  }

  function postComment() {
    var payload = Array.prototype.slice.call(commentForm)
      .reduce(function(result, x) {
        if(x.name) {
          if(x.type && x.type == "radio") {
            if(x.checked) {
              result[x.name] = x.value;
            }
          } else {
            result[x.name] = x.value;
          }
        }
        return result;
      }, {});

    xhr("POST", `${commentsURL}/api/${documentID}`, payload, (response) => displayCommentsFromJSON(response, payload.inReplyTo));
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
