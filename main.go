package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"log"
	"regexp"
)

const FILETYPE_KEY = "filetype"
const MB = 1024 * 1024

var Version string
var GitCommit string
var Verbose bool = false

var MinSizeForLargeFile = int64(50 * MB)

var REGEXES = map[string]*regexp.Regexp{
	"fastq": regexp.MustCompile(".*.fastq(.gz)?$"),
	"bam":   regexp.MustCompile(".*.bam$"),
}

func entityPath(obj *events.S3Entity) string {
	return fmt.Sprintf("s3://%s/%s", obj.Bucket.Name, obj.Object.Key)
}

func init() {
	log.SetPrefix(fmt.Sprintf("Version:%s|Commit:%s|", Version, GitCommit))
	if os.GetEnv("VERBOSE") != "" {
		Verbose = true
	}
}

type LambdaResponse struct {
	Count   int    `json:"count"`
	Message string `json:"message"`
}

func LambdaHandler(ctx context.Context, evt *events.S3Event) (*LambdaResponse, error) {
	debugLogf("%s", &evt.String())
	if sess, err := session.NewSession(); err != nil {
		return nil, err
	} else {
		return handleEvent(ctx, evt, s3.New(sess))
	}
}

func debugMarshal(x interface{}) string {
	if !Verbose {
		return ""
	}
	return json.MarshalIndent(x, "", " ")
}

func debugLogf(fmt string, args ...interface{}) {
	if Verbose {
		log.Printf(fmt, args...)
	}
}

// run the full lambda event handler
func handleEvent(ctx context.Context, evt *events.S3Event, client s3iface.S3API) (*LambdaResponse, error) {
	debugLogf("%s", debugMarshal(evt))
	tagsApplied := 0
	for _, rec := range evt.Records {
		tagForObject := getTagForObject(&rec.S3)
		if tagForObject != nil {
			output, err := client.PutObjectTaggingWithContext(ctx, tagForObject)
			if err != nil {
				return nil, err
			}
			debugLogf("successfully applied tag to %s (%#v)", entityPath(&rec.S3), output.String())
			tagsApplied++
		}
	}
	return &LambdaResponse{Count: tagsApplied, Message: "completed successfully"}, nil
}

// if the S3Entity matches one of our regexes, return the object tag(s) to apply
// if it's larger than MinSizeForLargeFile, apply largefile filetype
func getTagForObject(obj *events.S3Entity) *s3.PutObjectTaggingInput {
	var filetype string
	filetypeKey := FILETYPE_KEY
	for potentialFiletype, regex := range REGEXES {
		if regex.MatchString(obj.Object.Key) {
			filetype = potentialFiletype
			break
		}
	}
	if filetype == "" && obj.Object.Size >= MinSizeForLargeFile {
		filetype = "largefile"
	}
	if filetype != "" {
		var versionId *string = nil
		// cannot pass through empty string to aws
		if obj.Object.VersionID != "" {
			versionId = &obj.Object.VersionID
		}
		return &s3.PutObjectTaggingInput{
			Bucket: &obj.Bucket.Name,
			Key:    &obj.Object.Key,
			Tagging: &s3.Tagging{
				TagSet: []*s3.Tag{
					{Key: aws.String(filetypeKey), Value: aws.String(filetype)},
				},
			},
			VersionId: versionId,
		}
	}
	return nil
}

func main() {
	lambda.Start(LambdaHandler)
}
