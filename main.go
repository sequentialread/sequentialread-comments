package main

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	errors "git.sequentialread.com/forest/pkg-errors"
	"github.com/SYM01/htmlsanitizer"
	"github.com/boltdb/bolt"
	markdown "github.com/gomarkdown/markdown"
	markdown_to_html "github.com/gomarkdown/markdown/html"
)

type Comment struct {
	Email                 string        `json:"email"`
	Username              string        `json:"username"`
	Body                  string        `json:"body"`
	BodyHTML              template.HTML `json:"bodyHTML,omitempty"`
	UserID                string        `json:"userId"`
	GravatarHash          string        `json:"gravatarHash"`
	DocumentID            string        `json:"documentId"`
	Date                  int64         `json:"date"`
	GoogleCaptchaResponse string        `json:"g-recaptcha-response,omitempty"`
}

var origins []string
var portString = "$COMMENTS_LISTEN_PORT"
var recaptchaSiteKey = "$COMMENTS_RECAPTCHA_SITE_KEY"
var recaptchaSecretKey = "$COMMENTS_RECAPTCHA_SECRET_KEY"
var emailHost = "$COMMENTS_EMAIL_HOST"
var emailPort = "$COMMENTS_EMAIL_PORT"
var emailUsername = "$COMMENTS_EMAIL_USER"
var emailPassword = "$COMMENTS_EMAIL_PASSWORD"
var emailNotificationTarget = "$COMMENTS_NOTIFICATION_TARGET"
var adminPassword = "$COMMENTS_ADMIN_PASSWORD"
var recaptchaHost = "www.google.com"
var recaptchaPath = "/recaptcha/api/siteverify"
var db *bolt.DB
var httpClient *http.Client

var commentsTemplate *template.Template

var markdownRenderer *markdown_to_html.Renderer

//var indexTemplate *template.Template

func main() {

	portString = os.ExpandEnv(portString)
	portNumber, err := strconv.Atoi(portString)
	if err != nil {
		panic(errors.Wrap(err, "can't parse port number as int"))
	}
	originsCSV := os.ExpandEnv("$COMMENTS_CORS_ORIGINS")
	origins = splitNonEmpty(originsCSV, ",")
	recaptchaSiteKey = os.ExpandEnv(recaptchaSiteKey)
	recaptchaSecretKey = os.ExpandEnv(recaptchaSecretKey)
	emailHost = os.ExpandEnv(emailHost)
	emailPort = os.ExpandEnv(emailPort)
	emailUsername = os.ExpandEnv(emailUsername)
	emailPassword = os.ExpandEnv(emailPassword)
	emailNotificationTarget = os.ExpandEnv(emailNotificationTarget)
	adminPassword = os.ExpandEnv(adminPassword)

	db, err = bolt.Open("data/comments.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	httpClient = &http.Client{
		Timeout: time.Second * time.Duration(20),
	}

	markdownRenderer = markdown_to_html.NewRenderer(markdown_to_html.RendererOptions{
		Flags: markdown_to_html.CommonFlags | markdown_to_html.HrefTargetBlank,
	})

	commentsTemplate = loadTemplate("comments.html.gotemplate")

	http.HandleFunc("/api/", comments)

	http.HandleFunc("/admin/", admin)

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))

	http.ListenAndServe(fmt.Sprintf(":%d", portNumber), nil)
}

func comments(response http.ResponseWriter, request *http.Request) {
	addCORSHeaders(response, request)

	if request.Method == "OPTIONS" {
		response.WriteHeader(200)
		return
	}

	// TODO support hosting under a path rather than subdomain
	pathElements := splitNonEmpty(request.URL.Path, "/")
	//log.Printf("pathElements: [%s]", strings.Join(pathElements, ", "))
	var postID string
	if len(pathElements) != 2 {
		response.WriteHeader(404)
		response.Write([]byte("404 Not Found; postID is required"))
		return
	}
	postID = pathElements[1]
	if request.Method == "GET" {
		returnCommentsList(response, postID, "")
	} else if request.Method == "POST" {
		couldNotPostReason := postComment(response, request, postID)
		returnCommentsList(response, postID, couldNotPostReason)
	} else {
		response.Header().Add("Allow", "GET")
		response.Header().Add("Allow", "POST")
		response.Header().Add("Allow", "OPTIONS")
		response.WriteHeader(405)
		response.Write([]byte("405 Method Not Supported"))
	}
}

func admin(response http.ResponseWriter, request *http.Request) {
	addCORSHeaders(response, request)
	// response.WriteHeader(500)
	// response.Write([]byte("500 not implemented"))
	importComments(response, request)
}

func addCORSHeaders(response http.ResponseWriter, request *http.Request) {

	requestOrigin := request.Header.Get("Origin")

	for _, allowed := range origins {
		if allowed == requestOrigin {
			response.Header().Add("Access-Control-Allow-Origin", requestOrigin)
			for _, v := range request.Header.Values("access-control-request-method") {
				response.Header().Add("Access-Control-Allow-Methods", v)
			}
			for _, v := range request.Header.Values("access-control-request-headers") {
				response.Header().Add("Access-Control-Allow-Headers", v)
			}
			return
		}
	}

}

func postComment(response http.ResponseWriter, request *http.Request, postID string) string {
	var postedComment Comment
	requestBody, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Printf("http read error on post comment: %v", err)
		return "internal server error"
	}
	err = json.Unmarshal(requestBody, &postedComment)
	if err != nil {
		log.Printf("bad request: error reading posted comment: %v", err)
		return "bad request: malformed json"
	}
	err = validateGoogleCaptcha(postedComment.GoogleCaptchaResponse)
	if err != nil {
		log.Printf("validateGoogleCaptcha failed: %v", err)
		return "invalid captcha"
	}
	if regexp.MustCompile(`^[\s\t\n\r]*$`).MatchString(postedComment.Body) {
		return "comment body is required"
	}

	if postedComment.Email != "" {
		md5Hash := fmt.Sprintf("%x", md5.Sum([]byte(postedComment.Email)))
		postedComment.UserID = md5Hash[5:10]
		postedComment.GravatarHash = md5Hash
	}

	err = db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(fmt.Sprintf("posts/%s", postID)))
		if err != nil {
			return err
		}
		postedComment.DocumentID = postID
		postedComment.GoogleCaptchaResponse = ""
		postedComment.Date = getMillisecondsSinceUnixEpoch()
		postedBytes, err := json.Marshal(postedComment)
		if err != nil {
			return err
		}
		err = bucket.Put([]byte(fmt.Sprintf("%015d", postedComment.Date)), postedBytes)
		return err
	})
	if err != nil {
		log.Printf("boltdb write error on post comment: %v", err)
		return "boltdb write error"
	}
	return ""
}

func returnCommentsList(response http.ResponseWriter, postID, couldNotPostReason string) {
	comments := []Comment{}
	err := db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(fmt.Sprintf("posts/%s", postID)))
		if err != nil {
			return err
		}
		bucket.ForEach(func(k, v []byte) error {
			var comment Comment
			err := json.Unmarshal(v, &comment)
			if err != nil {
				return err
			}
			bodyHTML := string(markdown.ToHTML([]byte(comment.Body), nil, markdownRenderer))
			bodyHTML, err = htmlsanitizer.SanitizeString(bodyHTML)
			if err != nil {
				return err
			}
			comment.BodyHTML = template.HTML(bodyHTML)
			comments = append(comments, comment)
			return nil
		})
		return nil
	})
	if err != nil {
		log.Printf("boltdb read error: %v", err)
		response.WriteHeader(500)
		response.Write([]byte("boltdb read error"))
		return
	}

	commentsData := struct {
		RecaptchaSiteKey string
		Comments         []Comment
		Error            string
	}{
		RecaptchaSiteKey: recaptchaSiteKey,
		Comments:         comments,
		Error:            couldNotPostReason,
	}

	var buffer bytes.Buffer
	err = commentsTemplate.Execute(&buffer, commentsData)

	if err != nil {
		log.Printf("error templating comments: %v", err)
		response.WriteHeader(500)
		response.Write([]byte("500 internal server error"))
		return
	}

	io.Copy(response, &buffer)
}

func validateGoogleCaptcha(captchaResponse string) error {
	query := url.Values{}
	query.Add("secret", recaptchaSecretKey)
	query.Add("response", captchaResponse)

	recaptchaRequest, err := http.NewRequest(
		"POST",
		"https://www.google.com/recaptcha/api/siteverify",
		bytes.NewBuffer([]byte(query.Encode())),
	)
	if err != nil {
		return err
	}
	recaptchaRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := httpClient.Do(recaptchaRequest)
	if err != nil {
		return err
	}

	responseBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}

	googleResponse := struct {
		Success bool `json:"success"`
	}{}
	err = json.Unmarshal(responseBody, &googleResponse)
	if err != nil {
		return err
	}

	if !googleResponse.Success {
		return errors.New("captcha validation failed")
	}
	return nil
}

func loadTemplate(filename string) *template.Template {
	newTemplateString, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	newTemplate, err := template.New(filename).Parse(string(newTemplateString))
	if err != nil {
		panic(err)
	}
	return newTemplate
}

func splitNonEmpty(input, sep string) []string {
	toReturn := []string{}
	blah := strings.Split(input, sep)
	for _, s := range blah {
		if s != "" {
			toReturn = append(toReturn, s)
		}
	}
	return toReturn
}

func getMillisecondsSinceUnixEpoch() int64 {
	return time.Now().UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
}

func importComments(response http.ResponseWriter, request *http.Request) {

	bodyBytes, err := ioutil.ReadAll(request.Body)
	if err != nil {
		response.Write([]byte(fmt.Sprintf("%v", err)))
		return
	}
	var comments []Comment
	err = json.Unmarshal(bodyBytes, &comments)
	if err != nil {
		response.Write([]byte(fmt.Sprintf("%v", err)))
		return
	}

	for _, comment := range comments {
		err = db.Update(func(tx *bolt.Tx) error {
			bucket, err := tx.CreateBucketIfNotExists([]byte(fmt.Sprintf("posts/%s", comment.DocumentID)))
			if err != nil {
				return err
			}
			postedBytes, err := json.Marshal(comment)
			if err != nil {
				return err
			}
			err = bucket.Put([]byte(fmt.Sprintf("%015d", comment.Date)), postedBytes)
			return err
		})
		if err != nil {
			response.Write([]byte(fmt.Sprintf("%v", err)))
			return
		}
	}

	response.Write([]byte("ok"))
}
