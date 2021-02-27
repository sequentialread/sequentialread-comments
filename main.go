package main

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/png"
	"io/ioutil"
	"log"
	"math"
	mathRand "math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	errors "git.sequentialread.com/forest/pkg-errors"
	"github.com/SYM01/htmlsanitizer"
	"github.com/boltdb/bolt"
	markdown "github.com/gomarkdown/markdown"
	markdown_to_html "github.com/gomarkdown/markdown/html"
	mail "github.com/xhit/go-simple-mail"
)

type Comment struct {
	URL              string     `json:"url,omitempty"`
	DocumentTitle    string     `json:"documentTitle,omitempty"`
	AvatarType       string     `json:"avatarType,omitempty"`
	NotifyOfReplies  string     `json:"notifyOfReplies,omitempty"`
	Email            string     `json:"email,omitempty"`
	Username         string     `json:"username"`
	Body             string     `json:"body"`
	BodyHTML         string     `json:"bodyHTML,omitempty"`
	AvatarHash       string     `json:"avatarHash"`
	DocumentID       string     `json:"documentId"`
	InReplyTo        string     `json:"inReplyTo,omitempty"`
	Date             int64      `json:"date"`
	CaptchaChallenge string     `json:"captchaChallenge,omitempty"`
	CaptchaNonce     string     `json:"captchaNonce,omitempty"`
	Replies          []*Comment `json:"replies,omitempty"`
}

type CommentedDocument struct {
	URL           string `json:"url,omitempty"`
	DocumentTitle string `json:"documentTitle,omitempty"`
	Email         string `json:"email"`
	DocumentID    string `json:"documentId"`
}

var origins []string
var portString = "$COMMENTS_LISTEN_PORT"
var captchaAPIToken = "$COMMENTS_CAPTCHA_API_TOKEN"
var captchaAPIURLString = "$COMMENTS_CAPTCHA_URL"
var commentsBasePath = "$COMMENTS_BASE_PATH"
var commentsURLString = "$COMMENTS_BASE_URL"

// Note that every difficulty level is 16x more difficult than the last.
// Recommended difficulty level = 3
var captchaDifficultyLevelString = "$COMMENTS_CAPTCHA_DIFFICULTY_LEVEL"
var captchaDifficultyLevel int
var captchaAPIURL *url.URL
var loadCaptchaChallengesMutex *sync.Mutex
var captchaChallengesMutex *sync.Mutex
var loadCaptchaChallengesMutexIsProbablyLocked = false
var emailHost = "$COMMENTS_EMAIL_HOST"
var emailPort int
var emailUsername = "$COMMENTS_EMAIL_USER"
var emailPassword = "$COMMENTS_EMAIL_PASSWORD"
var emailNotificationTarget = "$COMMENTS_NOTIFICATION_TARGET"
var emailNotificationsDisabled = false
var adminPassword = "$COMMENTS_ADMIN_PASSWORD"
var hashSalt = "$COMMENTS_HASH_SALT"

var captchaChallenges []string
var db *bolt.DB
var httpClient *http.Client

var markdownRenderer *markdown_to_html.Renderer
var errBucketNotFound = errors.New("bucket not found")

func main() {

	commentsBasePath = os.ExpandEnv(commentsBasePath)
	if !strings.HasPrefix(commentsBasePath, "/") {
		commentsBasePath = fmt.Sprintf("/%s", commentsBasePath)
	}
	if strings.HasSuffix(commentsBasePath, "/") {
		commentsBasePath = strings.TrimSuffix(commentsBasePath, "/")
	}
	commentsURLString = os.ExpandEnv(commentsURLString)
	if strings.HasSuffix(commentsURLString, "/") {
		commentsURLString = strings.TrimSuffix(commentsURLString, "/")
	}
	if commentsURLString == "" {
		log.Printf("COMMENTS_BASE_URL is not set; email notifications will not work!")
		emailNotificationsDisabled = true
	}
	portString = os.ExpandEnv(portString)
	portNumber, err := strconv.Atoi(portString)
	if err != nil {
		panic(errors.Wrap(err, "can't parse port number as int"))
	}
	originsCSV := os.ExpandEnv("$COMMENTS_CORS_ORIGINS")
	origins = splitNonEmpty(originsCSV, ",")
	log.Printf("Allowed CORS Origins: [\n%s\n]\n", strings.Join(origins, "\n"))

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
	emailPortString := os.ExpandEnv("$COMMENTS_EMAIL_PORT")
	emailPort, err = strconv.Atoi(emailPortString)
	if err != nil {
		emailPort = 465
		log.Printf("WARNING: Could not parse $COMMENTS_EMAIL_PORT (%s) as an integer: %s email notifications will not work!", emailPortString, err)
		emailNotificationsDisabled = true
	}
	emailUsername = os.ExpandEnv(emailUsername)
	emailPassword = os.ExpandEnv(emailPassword)
	emailNotificationTarget = os.ExpandEnv(emailNotificationTarget)
	if emailHost == "" {
		log.Printf("COMMENTS_EMAIL_HOST is not set; email notifications will not work!")
		emailNotificationsDisabled = true
	}
	if emailUsername == "" {
		log.Printf("COMMENTS_EMAIL_USER is not set; email notifications will not work!")
		emailNotificationsDisabled = true
	}
	if emailPassword == "" {
		log.Printf("COMMENTS_EMAIL_PASSWORD is not set; email notifications will not work!")
		emailNotificationsDisabled = true
	}
	if emailNotificationTarget == "" {
		log.Printf("COMMENTS_NOTIFICATION_TARGET is not set; admin email notifications will not work!")
	}
	hashSalt = os.ExpandEnv(hashSalt)
	if hashSalt == "" {
		log.Printf("info: COMMENTS_HASH_SALT environment variable is not set. using the default value. for best practice, set this variable to a long random string")
		hashSalt = "983q4gh_8778g4ilb.sDkjg09834goj4p9-023u0_mjpmodsmg"
	}
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

	http.HandleFunc(fmt.Sprintf("%s/api/", commentsBasePath), comments)

	if adminPassword == "" {
		log.Println("WARNING: COMMENTS_ADMIN_PASSWORD environment variable was not set. The admin API will be turned off.")
	} else {
		http.HandleFunc(fmt.Sprintf("%s/admin/", commentsBasePath), admin)
	}

	http.HandleFunc(fmt.Sprintf("%s/avatar/", commentsBasePath), serveAvatar)

	http.HandleFunc(fmt.Sprintf("%s/disable/", commentsBasePath), disableNotification)

	http.HandleFunc(fmt.Sprintf("%s/unsubscribe/", commentsBasePath), unsubscribeNotification)

	//http.HandleFunc(fmt.Sprintf("%s/import/", commentsBasePath), importComments)

	staticPath := fmt.Sprintf("%s/static/", commentsBasePath)
	http.Handle(staticPath, http.StripPrefix(staticPath, http.FileServer(http.Dir("./static/"))))

	log.Printf(" 💬   SequentialRead Comments listening on ':%d', base path '%s'\n", portNumber, commentsBasePath)

	err = http.ListenAndServe(fmt.Sprintf(":%d", portNumber), nil)

	// if it got this far it means the server crashed!
	panic(err)
}

func comments(response http.ResponseWriter, request *http.Request) {
	addCORSHeaders(response, request)

	if request.Method == "OPTIONS" {
		response.WriteHeader(200)
		return
	}

	pathElements := splitNonEmpty(request.URL.Path, "/")
	if len(pathElements) < 2 {
		response.WriteHeader(404)
		response.Write([]byte("404 Not Found; postID is required"))
		return
	}
	postID := pathElements[len(pathElements)-1]
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

func admin(responseWriter http.ResponseWriter, request *http.Request) {
	username, password, ok := request.BasicAuth()
	if !ok || username != "admin" || password != adminPassword {
		log.Printf("admin api auth fail: '%s:******', ok=%t", username, ok)

		responseWriter.Header().Set("WWW-Authenticate", "Basic realm=\"comments admin\"")
		responseWriter.WriteHeader(401)
		responseWriter.Write([]byte("401 unauthorized"))
		return
	}

	pathSplit := splitNonEmpty(request.URL.Path, "/")
	var err error
	var templateBytes []byte
	var htmlTemplate *template.Template
	templateData := struct {
		Documents     []CommentedDocument
		DocumentTitle string
		Comments      []Comment
	}{
		Documents: []CommentedDocument{},
		Comments:  []Comment{},
	}
	templateBytes, err = ioutil.ReadFile("admin.html.gotemplate")
	if err == nil {
		htmlTemplate, err = template.New("admin").Parse(string(templateBytes))
	}
	if err != nil {
		log.Printf("failed to load admin.html.gotemplate: %v", err)
		responseWriter.WriteHeader(500)
		responseWriter.Write([]byte("500 internal server error"))
		return
	}

	if pathSplit[len(pathSplit)-1] == "admin" {
		err = db.Update(func(tx *bolt.Tx) error {
			bucket, err := tx.CreateBucketIfNotExists([]byte("posts_index"))
			if err != nil {
				return err
			}
			err = bucket.ForEach(func(k, v []byte) error {
				var document CommentedDocument
				err = json.Unmarshal(v, &document)
				if err != nil {
					return err
				}
				templateData.Documents = append(templateData.Documents, document)
				return nil
			})
			if err != nil {
				return err
			}
			return nil
		})
	} else {
		postID := pathSplit[len(pathSplit)-1]

		if request.Method == "POST" {
			err = request.ParseForm()
			if err == nil {
				date := request.Form.Get("date")
				var dateInt int64
				dateInt, err = strconv.ParseInt(date, 10, 64)
				if err == nil {
					err = db.Update(func(tx *bolt.Tx) error {
						bucket := tx.Bucket([]byte(fmt.Sprintf("posts/%s", postID)))
						if bucket == nil {
							return errBucketNotFound
						}
						bucket.Delete([]byte(fmt.Sprintf("%015d", dateInt)))
						return nil
					})
				}
			}
		}
		if err == nil {
			err = db.View(func(tx *bolt.Tx) error {
				bucket := tx.Bucket([]byte(fmt.Sprintf("posts/%s", postID)))
				if bucket == nil {
					return errBucketNotFound
				}
				err = bucket.ForEach(func(k, v []byte) error {
					var comment Comment
					err = json.Unmarshal(v, &comment)
					if err != nil {
						return err
					}
					if templateData.DocumentTitle == "" {
						templateData.DocumentTitle = comment.DocumentTitle
					}
					templateData.Comments = append(templateData.Comments, comment)
					return nil
				})
				if err != nil {
					return err
				}
				return nil
			})
		}
	}

	if err == errBucketNotFound {
		responseWriter.WriteHeader(404)
		responseWriter.Write([]byte("404 bucket not found"))
		return
	}

	var buffer bytes.Buffer
	if err == nil {
		err = htmlTemplate.Execute(&buffer, templateData)
	}

	if err != nil {
		log.Printf("failed to load admin api index page: %v", err)
		responseWriter.WriteHeader(500)
		responseWriter.Write([]byte("500 internal server error"))
		return
	}

	responseWriter.Write(buffer.Bytes())
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

	var avatarBytes []byte
	var avatarContentType string
	var sha256Hash string
	if postedComment.Email != "" {
		postedComment.Email = strings.ToLower(postedComment.Email)
		md5Hash := fmt.Sprintf("%x", md5.Sum([]byte(postedComment.Email)))
		saltedInput := fmt.Sprintf("%s%s", md5Hash, hashSalt)
		sha256Hash = fmt.Sprintf("%x", sha256.Sum256([]byte(saltedInput)))

		if postedComment.AvatarType == "gravatar" {
			response, err := httpClient.Get(fmt.Sprintf("https://www.gravatar.com/avatar/%s?d=retro", md5Hash))
			if err == nil && response.StatusCode == 200 {
				responseBytes, err := ioutil.ReadAll(response.Body)
				if err == nil {
					avatarBytes = responseBytes
					avatarContentType = response.Header.Get("Content-Type")
				}
			}
		} else {
			avatarBytes = generateIdenticonPNG(saltedInput)
			avatarContentType = "image/png"
		}
	}

	postedCommentDate := getMillisecondsSinceUnixEpoch()
	err = db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(fmt.Sprintf("posts/%s", postID)))
		if err != nil {
			return err
		}
		// fields that are computed on read
		postedComment.Replies = nil
		postedComment.BodyHTML = ""

		// metadata fields
		postedComment.AvatarType = ""
		postedComment.CaptchaChallenge = ""
		postedComment.CaptchaNonce = ""
		splitOnHash := strings.Split(postedComment.URL, "#")
		postedComment.URL = splitOnHash[0]
		// only save the email if the user requested it
		if postedComment.NotifyOfReplies == "" || postedComment.NotifyOfReplies == "off" {
			postedComment.Email = ""
		}

		// fields that are computed on write
		if sha256Hash != "" {
			postedComment.AvatarHash = sha256Hash[:6]
		}
		if postedComment.Username == "" {
			postedComment.Username = "Person Who Leaves Username Field Blank"
		}
		postedComment.DocumentID = postID
		postedComment.Date = postedCommentDate
		postedBytes, err := json.Marshal(postedComment)
		if err != nil {
			return err
		}
		err = bucket.Put([]byte(fmt.Sprintf("%015d", postedComment.Date)), postedBytes)
		if err != nil {
			return err
		}

		bucket, err = tx.CreateBucketIfNotExists([]byte("posts_index"))
		if err != nil {
			return err
		}
		err = bucket.Put([]byte(postID), postedBytes)
		if err != nil {
			return err
		}

		if avatarBytes != nil && len(avatarBytes) > 0 {
			bucket, err = tx.CreateBucketIfNotExists([]byte("avatars"))
			if err != nil {
				return err
			}

			err = bucket.Put([]byte(postedComment.AvatarHash), avatarBytes)
			if err != nil {
				return err
			}
			err = bucket.Put([]byte(fmt.Sprintf("%s_content-type", postedComment.AvatarHash)), []byte(avatarContentType))
		}
		return err
	})
	if err != nil {
		log.Printf("boltdb error on post comment: %v", err)
		return "database error"
	}

	if emailNotificationsDisabled {
		return ""
	}

	emailNotifications := map[string]*Comment{}

	err = db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(fmt.Sprintf("posts/%s", postID)))
		if bucket != nil {
			comments := map[string]*Comment{}
			rootComments := []*Comment{}
			bucket.ForEach(func(k, v []byte) error {
				var comment Comment
				err := json.Unmarshal(v, &comment)
				if err != nil {
					return err
				}
				comments[fmt.Sprintf("%s_%d", comment.DocumentID, comment.Date)] = &comment
				if comment.InReplyTo == "" || comment.InReplyTo == "root" {
					rootComments = append(rootComments, &comment)
				}
				return nil
			})

			siblingPosts := []*Comment{}
			parentPosts := []*Comment{}
			if postedComment.InReplyTo == "" || postedComment.InReplyTo == "root" {
				siblingPosts = rootComments
			} else {
				for _, comment := range comments {
					if comment.InReplyTo == postedComment.InReplyTo {
						siblingPosts = append(siblingPosts, comment)
					}
				}
				currentParent, hasCurrentParent := comments[postedComment.InReplyTo]
				for hasCurrentParent {
					parentPosts = append(parentPosts, currentParent)
					currentParent, hasCurrentParent = comments[currentParent.InReplyTo]
				}
			}

			emailDisables, err := tx.CreateBucketIfNotExists([]byte("email_disables"))
			if err != nil {
				return err
			}
			emailDocumentDisables, err := tx.CreateBucketIfNotExists([]byte("email_document_disables"))
			if err != nil {
				return err
			}

			notify := append(siblingPosts, parentPosts...)
			for _, comment := range notify {
				// dont notify the comment that was just posted about itself being posted!
				// don't notify users about thier own comments!
				if comment.Date == postedCommentDate || comment.AvatarHash == postedComment.AvatarHash {
					continue
				}

				// dont notify if this email address has been 100% unsubbed
				if emailDisables.Get([]byte(comment.Email)) != nil {
					continue
				}

				// dont notify if this email address has muted this document
				if emailDocumentDisables.Get([]byte(fmt.Sprintf("%s:%s", comment.Email, comment.DocumentID))) != nil {
					continue
				}

				emailSplit := strings.Split(comment.Email, "@")
				domainSplit := []string{}
				if len(emailSplit) == 2 {
					domainSplit = strings.Split(emailSplit[1], ".")
				}
				if comment.NotifyOfReplies == "child+sibling" && len(domainSplit) > 1 {
					emailNotifications[comment.Email] = comment
				}
			}
		}

		if len(emailNotifications) == 0 {
			return nil
		}

		emailDocumentNotificationsBucket, err := tx.CreateBucketIfNotExists([]byte("email_document_notifications"))
		if err != nil {
			log.Println("couldn't send email notifications because couldn't create/open bucket email_document_notifications")
			return nil
		}

		emailNotificationsBucket, err := tx.CreateBucketIfNotExists([]byte("email_notifications"))
		if err != nil {
			log.Println("couldn't send email notifications because couldn't create/open bucket email_notifications")
			return nil
		}

		for email, notifiedComment := range emailNotifications {
			unsubID := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%s%s", email, hashSalt))))[0:8]
			muteDocumentID := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%s%s%s", email, notifiedComment.DocumentID, hashSalt))))[0:8]

			muteDocument := CommentedDocument{
				URL:           notifiedComment.URL,
				DocumentID:    notifiedComment.DocumentID,
				DocumentTitle: notifiedComment.DocumentTitle,
				Email:         email,
			}
			muteDocumentBytes, err := json.Marshal(muteDocument)
			if err != nil {
				log.Printf("couldn't send email notification to %s because couldn't json.Marshal(muteDocument)", email)
				continue
			}

			err = emailNotificationsBucket.Put([]byte(unsubID), []byte(email))
			if err != nil {
				log.Printf("couldn't send email notification to %s because couldn't save unsubID", email)
				continue
			}
			err = emailDocumentNotificationsBucket.Put([]byte(muteDocumentID), muteDocumentBytes)
			if err != nil {
				log.Printf("couldn't send email notification to %s because couldn't save muteDocument", email)
				continue
			}

			go sendEmailNotification(email, &postedComment, notifiedComment, unsubID, muteDocumentID)
		}

		_, adminEmailIsAlreadyNotified := emailNotifications[emailNotificationTarget]
		if emailNotificationTarget != "" && !adminEmailIsAlreadyNotified {
			fakeAdminNotifiedComment := Comment{
				URL:           postedComment.URL,
				DocumentTitle: postedComment.DocumentTitle,
				Username:      "Admin",
			}
			go sendEmailNotification(emailNotificationTarget, &postedComment, &fakeAdminNotifiedComment, "admin_notification", "admin_notification")
		}

		return nil
	})

	return ""
}

var errAvatarNotFound = errors.New("avatar not found")

func serveAvatar(response http.ResponseWriter, request *http.Request) {
	addCORSHeaders(response, request)

	pathElements := splitNonEmpty(request.URL.Path, "/")
	if len(pathElements) < 2 {
		response.WriteHeader(404)
		response.Write([]byte("404 Not Found; avatarHash is required"))
		return
	}
	avatarHash := pathElements[len(pathElements)-1]

	var avatarBytes []byte
	var contentTypeBytes []byte
	err := db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("avatars"))
		if bucket == nil {
			return errAvatarNotFound
		}

		avatarBytes = bucket.Get([]byte(avatarHash))
		contentTypeBytes = bucket.Get([]byte(fmt.Sprintf("%s_content-type", avatarHash)))
		if avatarBytes == nil || contentTypeBytes == nil {
			return errAvatarNotFound
		}
		return nil
	})
	if err == errAvatarNotFound {
		response.WriteHeader(404)
		response.Write([]byte("404 Not Found"))
		return
	} else if err != nil {
		log.Printf("500 error in %s: %v", request.URL.Path, err)
		response.WriteHeader(500)
		response.Write([]byte("500 server error"))
		return
	}
	response.Header().Set("Content-Type", string(contentTypeBytes))
	response.Write(avatarBytes)
}

func returnCommentsList(response http.ResponseWriter, postID, couldNotPostReason string) {
	comments := map[string]*Comment{}
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
			comments[fmt.Sprintf("%s_%d", comment.DocumentID, comment.Date)] = &comment
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

	rootComments := []*Comment{}
	for _, comment := range comments {
		parentComment, has := comments[comment.InReplyTo]
		if has {
			parentComment.Replies = append(parentComment.Replies, comment)
		}
		if comment.InReplyTo == "" || comment.InReplyTo == "root" {
			rootComments = append(rootComments, comment)
		}
	}
	sortCommentSlice := func(slice []*Comment) {
		sort.Slice(slice, func(i, j int) bool {
			return slice[i].Date < slice[j].Date
		})
	}
	for _, comment := range comments {
		sortCommentSlice(comment.Replies)
	}
	sortCommentSlice(rootComments)

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
		CaptchaURL       string     `json:"captchaURL"`
		CaptchaChallenge string     `json:"captchaChallenge"`
		Comments         []*Comment `json:"comments"`
		Error            string     `json:"error"`
	}{
		CaptchaURL:       captchaAPIURL.String(),
		CaptchaChallenge: challenge,
		Comments:         rootComments,
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

func disableNotification(responseWriter http.ResponseWriter, request *http.Request) {
	pathSplit := splitNonEmpty(request.URL.Path, "/")
	disableID := pathSplit[len(pathSplit)-1]

	err := db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("email_document_notifications"))
		if err != nil {
			return err
		}
		disableNotificationBytes := bucket.Get([]byte(disableID))
		if disableNotificationBytes == nil {
			return fmt.Errorf("email_document_notifications %s not found", disableID)
		}
		var disableNotificationObj CommentedDocument
		err = json.Unmarshal(disableNotificationBytes, &disableNotificationObj)
		if err != nil {
			return err
		}
		bucket, err = tx.CreateBucketIfNotExists([]byte("email_document_disables"))
		if err != nil {
			return err
		}
		err = bucket.Put([]byte(fmt.Sprintf("%s:%s", disableNotificationObj.Email, disableNotificationObj.DocumentID)), []byte("true"))
		if err != nil {
			return err
		}
		responseWriter.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(responseWriter,
			"all notifications for %s on the document '%s' (%s) have been disabled",
			disableNotificationObj.Email, disableNotificationObj.DocumentTitle, disableNotificationObj.URL,
		)
		return nil
	})
	if err != nil {
		log.Printf("failed to disable notifications disableID=%s: %v", disableID, err)
		responseWriter.Header().Set("Content-Type", "text/plain")
		responseWriter.WriteHeader(500)
		responseWriter.Write([]byte("500 internal server error"))
	}
}

func unsubscribeNotification(responseWriter http.ResponseWriter, request *http.Request) {
	pathSplit := splitNonEmpty(request.URL.Path, "/")
	unsubID := pathSplit[len(pathSplit)-1]

	err := db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("email_notifications"))
		if err != nil {
			return err
		}
		emailBytes := bucket.Get([]byte(unsubID))
		if emailBytes == nil {
			return fmt.Errorf("email_notifications %s not found", unsubID)
		}
		bucket, err = tx.CreateBucketIfNotExists([]byte("email_disables"))
		if err != nil {
			return err
		}
		err = bucket.Put(emailBytes, []byte("true"))
		if err != nil {
			return err
		}
		responseWriter.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(responseWriter, "%s has been unsubscribed from all notifications", string(emailBytes))
		return nil
	})
	if err != nil {
		log.Printf("failed to unsubscribe unsubID=%s: %v", unsubID, err)
		responseWriter.Header().Set("Content-Type", "text/plain")
		responseWriter.WriteHeader(500)
		responseWriter.Write([]byte("500 internal server error"))
	}
}

func sendEmailNotification(email string, postedComment, notifiedComment *Comment, unsubID, muteDocumentID string) {
	addressedTo := notifiedComment.Username
	if addressedTo == "" {
		addressedTo = "Commenter"
	}
	other := postedComment.Username
	if other == "" {
		addressedTo = "Someone"
	}
	disableArticleLink := fmt.Sprintf("%s/disable/%s", commentsURLString, muteDocumentID)
	unsubscribeLink := fmt.Sprintf("%s/unsubscribe/%s", commentsURLString, unsubID)
	htmlEscapedBody := strings.ReplaceAll(postedComment.Body, "<", "&lt;")
	htmlEscapedBody = strings.ReplaceAll(htmlEscapedBody, ">", "&gt;")
	bodyPlain := fmt.Sprintf(
		`%s,

%s posted a reply on the article 

'%s' 

at: %s


%s

---------------------------------------------------------------------

To disable notifications for future comments on this article, please 
visit the following link in your web browser:

%s

If you believe you have recieved this message in error, or to 
completely unsubscribe from all email from this service, please visit 
the following link in your web browser:

%s


Powered by SequentialRead Comments: https://git.sequentialread.com/forest/sequentialread-comments
`, addressedTo, other, notifiedComment.DocumentTitle, notifiedComment.URL, postedComment.Body, disableArticleLink, unsubscribeLink)

	bodyPlain = softWrapString(bodyPlain, 72)

	bodyHTML := fmt.Sprintf(
		`%s,<br/>
<br/>
%s <a href="%s#%s_%d">posted a reply</a> on the article <a href="%s">%s</a>:<br/>
<br/>
%s<br/>
<br/>
<br/>
<div style="padding:2em; border-top: 1px solid #aaa;">
<span style="font-size:0.9em">If you believe you have recieved this message in error, please click the unsubscribe link below.</span><br/>
<br/>
<a style="font-size:0.9em" href="%s">disable notifications for future comments on this article</a>
| <a style="font-size:0.9em" href="%s">completely unsubscribe from all email from this service</a><br/>
<br/>
<br/>
Powered by <a style="font-size:0.9em" href="https://git.sequentialread.com/forest/sequentialread-comments">SequentialRead Comments</a>
</div>

`, addressedTo, other, notifiedComment.URL, postedComment.DocumentID, postedComment.Date,
		notifiedComment.URL, notifiedComment.DocumentTitle, htmlEscapedBody, disableArticleLink, unsubscribeLink)

	err := sendEmail(email, fmt.Sprintf("New Reply on '%s'", notifiedComment.DocumentTitle), bodyPlain, bodyHTML)
	if err != nil {
		log.Printf("email delivery issue for %s: %v", email, err)
	}
}

func softWrapString(text string, columns int) string {

	lines := strings.Split(text, "\n")
	newLines := []string{}
	for _, line := range lines {
		whitespaces := []int{}
		runes := []rune(fmt.Sprintf("%s ", line))
		for j := 0; j < len(runes); j++ {
			if runes[j] == ' ' {
				whitespaces = append(whitespaces, j)
			}
		}
		offset := 0
		for j := 0; j < len(whitespaces); j++ {
			if j < len(whitespaces)-1 {
				if whitespaces[j]-offset < columns && whitespaces[j+1]-offset > columns {
					offset = whitespaces[j]
					runes[whitespaces[j]] = '\n'
				}
			}
		}
		newLines = append(newLines, string(runes))
	}
	return strings.Join(newLines, "\n")

}

func sendEmail(to, subject, bodyPlain, bodyHTML string) error {
	smtpClient := mail.NewSMTPClient()
	smtpClient.Host = emailHost
	smtpClient.Port = emailPort
	smtpClient.Username = emailUsername
	smtpClient.Password = emailPassword
	if emailPort == 465 {
		smtpClient.Encryption = mail.EncryptionSSL
	} else {
		smtpClient.Encryption = mail.EncryptionTLS
	}
	smtpClient.KeepAlive = false
	smtpClient.ConnectTimeout = 10 * time.Second
	smtpClient.SendTimeout = 10 * time.Second

	smtpConn, err := smtpClient.Connect()

	if err != nil {
		return err
	}

	email := mail.NewMSG()

	email.SetFrom(emailUsername)
	email.AddTo(to)
	email.SetSubject(subject)
	email.SetBody(mail.TextPlain, bodyPlain)
	email.AddAlternative(mail.TextHTML, bodyHTML)

	err = email.Send(smtpConn)

	if err != nil {
		return err
	}
	return nil
}

func generateIdenticonPNG(input string) []byte {
	var randomInt int64
	colorHashArray := md5.Sum([]byte(input))
	colorHash := bytes.NewReader(colorHashArray[0:16])
	err := binary.Read(colorHash, binary.LittleEndian, &randomInt)
	if err != nil {
		panic(err)
	}
	randomSource := mathRand.NewSource(randomInt)

	baseHue := float64(randomSource.Int63() % 360)
	highlightHue := float64(randomSource.Int63() % 360)
	uglyStart := baseHue - 160
	uglyEnd := baseHue
	if baseHue > 120 && baseHue < 240 {
		uglyStart = baseHue
		uglyEnd = baseHue + 160
	}
	isInUglyArea := func(hue float64) bool {
		wrap1 := hue + 360
		wrap2 := hue - 360
		return (hue > uglyStart && hue < uglyEnd) || (wrap1 > uglyStart && wrap1 < uglyEnd) || (wrap2 > uglyStart && wrap2 < uglyEnd)
	}
	for i := 0; isInUglyArea(highlightHue) && i < 100; i++ {
		highlightHue = float64(randomSource.Int63() % 360)
		if i == 99 {
			log.Println("Something might be wrong with the ugly color prevention! it's looping 100 times!")
		}
	}

	baseColor := HSVColor(
		baseHue,
		float64(0.68)+(float64(randomSource.Int63()%80)/float64(255)),
		float64(0.10)+(float64(randomSource.Int63()%50)/float64(255)),
	)
	highlightColor := HSVColor(
		highlightHue,
		float64(0.47)+(float64(randomSource.Int63()%80)/float64(255)),
		float64(0.6)+(float64(randomSource.Int63()%80)/float64(255)),
	)

	size := 10
	pixelsPerSquare := 8
	thresholdForHighlight := float64(0.85)
	previousRowFudgeFactor := float64(0.35)
	img := image.NewRGBA(image.Rectangle{image.Point{0, 0}, image.Point{size * pixelsPerSquare, size * pixelsPerSquare}})

	width := size / 2
	squares := make([]bool, size*width)
	wrap := func(v int, mod int) int {
		if v < 0 {
			return v + mod
		}
		return v % mod
	}
	getContributionFromPreviousRow := func(y, x int) int {
		previousRowContribution := 0
		if y > 0 {
			if squares[(y-1)*width+wrap(x-1, width)] {
				previousRowContribution++
			}
			if squares[(y-1)*width+x] {
				previousRowContribution++
			}
			if squares[(y-1)*width+wrap(x+1, width)] {
				previousRowContribution++
			}
		}
		return previousRowContribution
	}
	for y := 0; y < size; y++ {
		contributionFromPreviousRow := 0
		for x := 0; x < width; x++ {
			contributionFromPreviousRow += getContributionFromPreviousRow(y, x)
		}
		averageContributionFromPreviousRow := float64(contributionFromPreviousRow) / float64(width)
		for x := 0; x < width; x++ {
			previousRowContribution := getContributionFromPreviousRow(y, x)
			normalizedPreviousRowContribution := float64(0.2)
			if averageContributionFromPreviousRow > 0 {
				normalizedPreviousRowContribution = float64(previousRowContribution) / averageContributionFromPreviousRow
			}
			normalizedPreviousRowContribution *= previousRowFudgeFactor
			randomContribution := float64(randomSource.Int63()%1000) / float64(1000)
			squares[(y*width)+x] = normalizedPreviousRowContribution+randomContribution > thresholdForHighlight
		}
	}

	for x := 0; x < size*pixelsPerSquare; x++ {
		for y := 0; y < size*pixelsPerSquare; y++ {
			usedX := x / pixelsPerSquare
			if usedX >= width {
				usedX = -usedX + ((width * 2) - 1)
			}
			if squares[((y/pixelsPerSquare)*width)+usedX] {
				img.Set(x, y, highlightColor)
			} else {
				img.Set(x, y, baseColor)
			}
		}
	}

	var buffer bytes.Buffer
	png.Encode(&buffer, img)

	return []byte(buffer.Bytes())
}

func HSVColor(H, S, V float64) color.RGBA {
	Hp := H / 60.0
	C := V * S
	X := C * (1.0 - math.Abs(math.Mod(Hp, 2.0)-1.0))

	m := V - C
	r, g, b := 0.0, 0.0, 0.0

	switch {
	case 0.0 <= Hp && Hp < 1.0:
		r = C
		g = X
	case 1.0 <= Hp && Hp < 2.0:
		r = X
		g = C
	case 2.0 <= Hp && Hp < 3.0:
		g = C
		b = X
	case 3.0 <= Hp && Hp < 4.0:
		g = X
		b = C
	case 4.0 <= Hp && Hp < 5.0:
		r = X
		b = C
	case 5.0 <= Hp && Hp < 6.0:
		r = C
		b = X
	}

	return color.RGBA{uint8(int((m + r) * float64(255))), uint8(int((m + g) * float64(255))), uint8(int((m + b) * float64(255))), 0xff}
}

// func importComments(responseWriter http.ResponseWriter, request *http.Request) {

// 	bodyBytes, err := ioutil.ReadAll(request.Body)
// 	if err != nil {
// 		responseWriter.Write([]byte(fmt.Sprintf("%v", err)))
// 		return
// 	}
// 	var comments []*Comment
// 	err = json.Unmarshal(bodyBytes, &comments)
// 	if err != nil {
// 		responseWriter.Write([]byte(fmt.Sprintf("%v", err)))
// 		return
// 	}

// 	for _, postedComment := range comments {

// 		postID := postedComment.DocumentID
// 		var avatarBytes []byte
// 		var avatarContentType string
// 		var sha256Hash string

// 		if postedComment.AvatarHash != "" {
// 			md5Hash := postedComment.AvatarHash
// 			saltedInput := fmt.Sprintf("%s%s", md5Hash, hashSalt)
// 			sha256Hash = fmt.Sprintf("%x", sha256.Sum256([]byte(saltedInput)))

// 			response, err := httpClient.Get(fmt.Sprintf("https://www.gravatar.com/avatar/%s?d=retro", md5Hash))
// 			if err == nil && response.StatusCode == 200 {
// 				responseBytes, err := ioutil.ReadAll(response.Body)
// 				if err == nil {
// 					avatarBytes = responseBytes
// 					avatarContentType = response.Header.Get("Content-Type")
// 				}
// 			}
// 		}

// 		err = db.Update(func(tx *bolt.Tx) error {
// 			bucket, err := tx.CreateBucketIfNotExists([]byte(fmt.Sprintf("posts/%s", postID)))
// 			if err != nil {
// 				return err
// 			}

// 			// fields that are computed on read
// 			postedComment.Replies = nil
// 			postedComment.BodyHTML = ""

// 			// metadata fields
// 			postedComment.AvatarType = ""
// 			postedComment.CaptchaChallenge = ""
// 			postedComment.CaptchaNonce = ""

// 			// fields that are computed on write
// 			if sha256Hash != "" {
// 				postedComment.AvatarHash = sha256Hash[:6]
// 			}

// 			postedBytes, err := json.Marshal(postedComment)
// 			if err != nil {
// 				return err
// 			}
// 			err = bucket.Put([]byte(fmt.Sprintf("%015d", postedComment.Date)), postedBytes)
// 			if err != nil {
// 				return err
// 			}

// 			bucket, err = tx.CreateBucketIfNotExists([]byte("posts_index"))
// 			if err != nil {
// 				return err
// 			}
// 			err = bucket.Put([]byte(postID), postedBytes)
// 			if err != nil {
// 				return err
// 			}

// 			if avatarBytes != nil && len(avatarBytes) > 0 {
// 				bucket, err = tx.CreateBucketIfNotExists([]byte("avatars"))
// 				if err != nil {
// 					return err
// 				}
// 				err = bucket.Put([]byte(postedComment.AvatarHash), avatarBytes)
// 				if err != nil {
// 					return err
// 				}
// 				err = bucket.Put([]byte(fmt.Sprintf("%s_content-type", postedComment.AvatarHash)), []byte(avatarContentType))
// 			}
// 			return err
// 		})

// 		if err != nil {
// 			responseWriter.Write([]byte(fmt.Sprintf("%v", err)))
// 			return
// 		}
// 	}

// 	responseWriter.Write([]byte("ok"))
// }
