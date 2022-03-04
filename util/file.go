package util

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/globalsign/mgo/bson"
)

func UploadFile(accessKey, secretKey, endpoint, bucket, region string, plot io.WriterTo) (string, error) {
	s := session.Must(session.NewSessionWithOptions(session.Options{
		Config: aws.Config{

			Endpoint:    aws.String(endpoint),
			Region:      aws.String(region),
			DisableSSL:  aws.Bool(true),
			Credentials: credentials.NewStaticCredentials(accessKey, secretKey, ""),
		},
	}))
	_, err := s.Config.Credentials.Get()

	f, err := ioutil.TempFile("", "promoter-*.png")
	if err != nil {
		return "", fmt.Errorf("failed to create tmp file: %v", err)
	}
	defer func() {
		err = f.Close()
		if err != nil {
			panic(fmt.Errorf("failed to close tmp file: %v", err))
		}
		err := os.Remove(f.Name())
		if err != nil {
			panic(fmt.Errorf("failed to delete tmp file: %v", err))
		}
	}()
	_, err = plot.WriteTo(f)
	if err != nil {
		return "", fmt.Errorf("failed to write plot to file: %v", err)
	}

	// get the file size and read
	// the file content into a buffer
	fileInfo, _ := f.Stat()
	size := fileInfo.Size()
	buffer := make([]byte, size)
	_, err = f.Seek(0, io.SeekStart)
	_, err = f.Read(buffer)

	// create a unique file name for the file
	tempFileName := "pictures/" + bson.NewObjectId().Hex() + "_" + strconv.FormatInt(time.Now().Unix(), 10) + ".png"

	_, err = s3.New(s).PutObject(&s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(tempFileName),
		ACL:           aws.String("public-read"),
		Body:          bytes.NewReader(buffer),
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(http.DetectContentType(buffer)),
	})
	if err != nil {
		return "", err
	}
	//http://my-oss-testing.oss-cn-beijing.aliyuncs.com/La6Z6PIeBMcCOX7cWoYz.png
	//return fmt.Sprintf("https://%s.s3-%s.amazonaws.com/%s", bucket, region, tempFileName), err
	return fmt.Sprintf("http://%s.%s/%s", bucket, endpoint, tempFileName), err
}
