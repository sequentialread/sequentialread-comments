package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	errors "git.sequentialread.com/forest/pkg-errors"
	"github.com/SYM01/htmlsanitizer"
	"github.com/boltdb/bolt"
	markdown "github.com/gomarkdown/markdown"
	markdown_to_html "github.com/gomarkdown/markdown/html"
)

type Comment struct {
	Email            string `json:"email"`
	Username         string `json:"username"`
	Body             string `json:"body"`
	BodyHTML         string `json:"bodyHTML,omitempty"`
	UserID           string `json:"userId"`
	GravatarHash     string `json:"gravatarHash"`
	DocumentID       string `json:"documentId"`
	Date             int64  `json:"date"`
	CaptchaChallenge string `json:"captchaChallenge,omitempty"`
	CaptchaNonce     string `json:"captchaNonce,omitempty"`
}

var origins []string
var portString = "$COMMENTS_LISTEN_PORT"
var captchaAPIToken = "$COMMENTS_CAPTCHA_API_TOKEN"
var captchaAPIURLString = "$COMMENTS_CAPTCHA_URL"

// Note that every difficulty level is 16x more difficult than the last.
// Recommended difficulty level = 3
var captchaDifficultyLevelString = "$COMMENTS_CAPTCHA_DIFFICULTY_LEVEL"
var captchaDifficultyLevel int
var captchaAPIURL *url.URL
var loadCaptchaChallengesMutex *sync.Mutex
var captchaChallengesMutex *sync.Mutex
var loadCaptchaChallengesMutexIsProbablyLocked = false
var emailHost = "$COMMENTS_EMAIL_HOST"
var emailPort = "$COMMENTS_EMAIL_PORT"
var emailUsername = "$COMMENTS_EMAIL_USER"
var emailPassword = "$COMMENTS_EMAIL_PASSWORD"
var emailNotificationTarget = "$COMMENTS_NOTIFICATION_TARGET"
var adminPassword = "$COMMENTS_ADMIN_PASSWORD"
var captchaChallenges []string
var db *bolt.DB
var httpClient *http.Client

var markdownRenderer *markdown_to_html.Renderer

func main() {

	portString = os.ExpandEnv(portString)
	portNumber, err := strconv.Atoi(portString)
	if err != nil {
		panic(errors.Wrap(err, "can't parse port number as int"))
	}
	originsCSV := os.ExpandEnv("$COMMENTS_CORS_ORIGINS")
	origins = splitNonEmpty(originsCSV, ",")
	captchaAPIToken = os.ExpandEnv(captchaAPIToken)
	captchaAPIURLString = os.ExpandEnv(captchaAPIURLString)
	captchaAPIURL, err = url.Parse(captchaAPIURLString)
	if err != nil {
		panic(errors.Wrapf(err, "can't parse COMMENTS_CAPTCHA_URL '%s' as url", captchaAPIURLString))
	}

	captchaDifficultyLevelString = os.ExpandEnv(captchaDifficultyLevelString)
	captchaDifficultyLevel, err = strconv.Atoi(captchaDifficultyLevelString)
	if err != nil {
		panic(errors.Wrapf(err, "can't parse COMMENTS_CAPTCHA_DIFFICULTY_LEVEL '%s' as int", captchaDifficultyLevelString))
	}
	loadCaptchaChallengesMutex = &sync.Mutex{}
	captchaChallengesMutex = &sync.Mutex{}

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

	err = loadCaptchaChallenges()
	if err != nil {
		panic(errors.Wrap(err, "could not loadCaptchaChallenges():"))
	}

	markdownRenderer = markdown_to_html.NewRenderer(markdown_to_html.RendererOptions{
		Flags: markdown_to_html.CommonFlags | markdown_to_html.HrefTargetBlank,
	})

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
	response.WriteHeader(500)
	response.Write([]byte("500 not implemented"))
	//importComments(response, request)
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
	err = validateCaptcha(postedComment.CaptchaChallenge, postedComment.CaptchaNonce)
	if err != nil {
		log.Printf("validateCaptcha failed: %v", err)
		return "proof of work captcha failed"
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
		postedComment.CaptchaChallenge = ""
		postedComment.CaptchaNonce = ""
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
			comment.BodyHTML = bodyHTML
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

	// if it looks like we will run out of challenges soon & not currently busy getting them,
	// then kick off a goroutine to go get them in the background.
	if len(captchaChallenges) > 0 && len(captchaChallenges) < 5 && !loadCaptchaChallengesMutexIsProbablyLocked {
		go loadCaptchaChallenges()
	}

	if captchaChallenges == nil || len(captchaChallenges) == 0 {
		err = loadCaptchaChallenges()
		if err != nil {
			log.Printf("loading captcha challenges failed: %v", err)
			response.WriteHeader(500)
			response.Write([]byte("captcha api error"))
			return
		}
	}
	var challenge string
	captchaChallengesMutex.Lock()
	challenge = captchaChallenges[0]
	captchaChallenges = captchaChallenges[1:]
	captchaChallengesMutex.Unlock()

	commentsData := struct {
		CaptchaURL       string    `json:"captchaURL"`
		CaptchaChallenge string    `json:"captchaChallenge"`
		Comments         []Comment `json:"comments"`
		Error            string    `json:"error"`
	}{
		CaptchaURL:       captchaAPIURL.String(),
		CaptchaChallenge: challenge,
		Comments:         comments,
		Error:            couldNotPostReason,
	}

	responseBytes, err := json.Marshal(commentsData)
	if err != nil {
		log.Printf("json marshal error: %v", err)
		response.WriteHeader(500)
		response.Write([]byte("json marshal error"))
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(200)
	response.Write(responseBytes)
}

func loadCaptchaChallenges() error {
	// make sure we only call this function once at a time.
	loadCaptchaChallengesMutex.Lock()
	loadCaptchaChallengesMutexIsProbablyLocked = true
	defer (func() {
		loadCaptchaChallengesMutexIsProbablyLocked = false
		loadCaptchaChallengesMutex.Unlock()
	})()

	query := url.Values{}
	query.Add("difficultyLevel", strconv.Itoa(captchaDifficultyLevel))

	loadURL := url.URL{
		Scheme:   captchaAPIURL.Scheme,
		Host:     captchaAPIURL.Host,
		Path:     filepath.Join(captchaAPIURL.Path, "GetChallenges"),
		RawQuery: query.Encode(),
	}

	captchaRequest, err := http.NewRequest("POST", loadURL.String(), nil)
	if err != nil {
		return err
	}
	captchaRequest.Header.Set("Authorization", fmt.Sprintf("Bearer %s", captchaAPIToken))

	response, err := httpClient.Do(captchaRequest)
	if err != nil {
		return err
	}

	responseBytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}

	if response.StatusCode != 200 {
		return fmt.Errorf(
			"load proof of work captcha challenges api returned http %d: %s",
			response.StatusCode, string(responseBytes),
		)
	}

	err = json.Unmarshal(responseBytes, &captchaChallenges)
	if err != nil {
		return err
	}

	if len(captchaChallenges) == 0 {
		return errors.New("proof of work captcha challenges api returned empty array")
	}

	return nil
}

func validateCaptcha(challenge, nonce string) error {
	query := url.Values{}
	query.Add("challenge", challenge)
	query.Add("nonce", nonce)
	query.Add("token", captchaAPIToken)

	verifyURL := url.URL{
		Scheme:   captchaAPIURL.Scheme,
		Host:     captchaAPIURL.Host,
		Path:     filepath.Join(captchaAPIURL.Path, "Verify"),
		RawQuery: query.Encode(),
	}

	captchaRequest, err := http.NewRequest("POST", verifyURL.String(), nil)
	if err != nil {
		return err
	}
	captchaRequest.Header.Set("Authorization", fmt.Sprintf("Bearer %s", captchaAPIToken))

	response, err := httpClient.Do(captchaRequest)
	if err != nil {
		return err
	}

	if response.StatusCode != 200 {
		return errors.New("proof of work captcha validation failed")
	}
	return nil
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

// func importComments(response http.ResponseWriter, request *http.Request) {

// 	bodyBytes, err := ioutil.ReadAll(request.Body)
// 	if err != nil {
// 		response.Write([]byte(fmt.Sprintf("%v", err)))
// 		return
// 	}
// 	var comments []Comment
// 	err = json.Unmarshal(bodyBytes, &comments)
// 	if err != nil {
// 		response.Write([]byte(fmt.Sprintf("%v", err)))
// 		return
// 	}

// 	for _, comment := range comments {
// 		err = db.Update(func(tx *bolt.Tx) error {
// 			bucket, err := tx.CreateBucketIfNotExists([]byte(fmt.Sprintf("posts/%s", comment.DocumentID)))
// 			if err != nil {
// 				return err
// 			}
// 			postedBytes, err := json.Marshal(comment)
// 			if err != nil {
// 				return err
// 			}
// 			err = bucket.Put([]byte(fmt.Sprintf("%015d", comment.Date)), postedBytes)
// 			return err
// 		})
// 		if err != nil {
// 			response.Write([]byte(fmt.Sprintf("%v", err)))
// 			return
// 		}
// 	}

// 	response.Write([]byte("ok"))
// }
