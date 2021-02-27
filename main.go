package main

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
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
)

type Comment struct {
	AvatarType       string     `json:"avatarType"`
	NotifyOfReplies  string     `json:"notifyOfReplies"`
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

	http.HandleFunc("/avatar/", serveAvatar)

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))

	http.ListenAndServe(fmt.Sprintf(":%d", portNumber), nil)
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
		md5Hash := fmt.Sprintf("%x", md5.Sum([]byte(strings.ToLower(postedComment.Email))))
		salt := "983q4gh_8778g4ilb.sDkjg09834goj4p9-023u0_mjpmodsmg"
		saltedInput := fmt.Sprintf("%s%s", md5Hash, salt)
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
		// only save the email if the user requested it
		if postedComment.NotifyOfReplies == "" || postedComment.NotifyOfReplies == "off" {
			postedComment.Email = ""
		}

		// fields that are computed on write
		if sha256Hash != "" {
			postedComment.AvatarHash = sha256Hash[:6]
		}
		postedComment.DocumentID = postID
		postedComment.Date = getMillisecondsSinceUnixEpoch()
		postedBytes, err := json.Marshal(postedComment)
		if err != nil {
			return err
		}
		err = bucket.Put([]byte(fmt.Sprintf("%015d", postedComment.Date)), postedBytes)
		if err != nil {
			return err
		}

		if avatarBytes != nil && len(avatarBytes) > 0 {
			bucket, err = tx.CreateBucketIfNotExists([]byte("avatars"))
			if err != nil {
				return err
			}

			log.Printf("PUT1: %s\n", postedComment.AvatarHash)
			log.Printf("PUT2: %s\n", base64.StdEncoding.EncodeToString(avatarBytes))
			log.Printf("PUT3: %s\n", avatarContentType)
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
		log.Printf("GET: %s", avatarHash)
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

	for _, postedComment := range comments {

		postID := postedComment.DocumentID
		var avatarBytes []byte
		var avatarContentType string
		var sha256Hash string
		if postedComment.Email != "" {
			md5Hash := postedComment.AvatarHash
			salt := "983q4gh_8778g4ilb.sDkjg09834goj4p9-023u0_mjpmodsmg"
			sha256Hash = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s%s", md5Hash, salt))))

			response, err := httpClient.Get(fmt.Sprintf("https://www.gravatar.com/avatar/%s?d=retro", md5Hash))
			if err == nil && response.StatusCode == 200 {
				responseBytes, err := ioutil.ReadAll(response.Body)
				if err == nil {
					avatarBytes = responseBytes
					avatarContentType = response.Header.Get("Content-Type")
				}
			}
		}

		err = db.Update(func(tx *bolt.Tx) error {
			bucket, err := tx.CreateBucketIfNotExists([]byte(fmt.Sprintf("posts/%s", postID)))
			if err != nil {
				return err
			}
			// fields that are computed on read
			postedComment.Replies = nil
			postedComment.BodyHTML = ""

			// metadata fields
			postedComment.CaptchaChallenge = ""
			postedComment.CaptchaNonce = ""
			// only save the email if the user requested it
			if postedComment.NotifyOfReplies == "" || postedComment.NotifyOfReplies == "off" {
				postedComment.Email = ""
			}

			// fields that are computed on write
			if sha256Hash != "" {
				postedComment.AvatarHash = sha256Hash[0:6]
			}
			postedComment.DocumentID = postID
			postedComment.Date = getMillisecondsSinceUnixEpoch()
			postedBytes, err := json.Marshal(postedComment)
			if err != nil {
				return err
			}
			err = bucket.Put([]byte(fmt.Sprintf("%015d", postedComment.Date)), postedBytes)
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
			response.Write([]byte(fmt.Sprintf("%v", err)))
			return
		}
	}

	response.Write([]byte("ok"))
}
