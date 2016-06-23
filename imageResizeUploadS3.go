/*
  imageResizeUploadS3.go
  go-image-resize-upload

  Created by Juan Roca on 2015-08-13.
  Copyright © 2015 Juan Roca. All rights reserved.
*/

package main

import (
	"log"
	"net/http"
	"io"
	"os"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"errors"
	"time"
	"strconv"
	"path/filepath"
	"bytes"
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/awserr"
    "github.com/aws/aws-sdk-go/aws/awsutil"
    "github.com/aws/aws-sdk-go/aws/credentials"
    "github.com/aws/aws-sdk-go/service/s3"
    "github.com/gographics/imagick/imagick"

)

var (
	Bucket 				= "bucket-name-goes-here"
	AccessKeyID			= "access-key"
	SecretAccessKey		= "secret-access-key"
)

const port string = "3000"

type Image struct {
	url string 
	extension string
	width string
	height string
	md5Key string
	fileName string
}

func main() {
	// Processing Channel
	processing := make(chan Image, 10)

	//startS3Connection()
	// Creates 5 poolWorker goroutines
	for w := 1; w <=5; w++ {
		go poolWorker(w, processing)
	}
	
	imageServer := http.NewServeMux()
	imageServer.Handle("/r", startIRService(processing))
	
	http.ListenAndServe(":" + port, imageServer)
}

// Resize image (gographics/imagick). Creates new resized file.
func (i Image) resizeImage() error {
	fmt.Println("Resizing....")
	imagick.Initialize()
	defer imagick.Terminate()

	mw := imagick.NewMagickWand()
	defer mw.Destroy()

	resizedFile, err := os.Create(i.md5Key + "_r" + i.extension)
	if err != nil {
		panic(err)
		return errors.New("Error creating resized file.")
	}

	defer resizedFile.Close()

	err = mw.ReadImage(i.fileName)
	if err != nil {
		panic(err)
		return errors.New("Error reading image file with MagicWand.")
	}
	/*
	Troubleshooting image types. Uncomment this too test the types.
	fmt.Println("8888888888888888888888888888888888888888888888888888888888888888888888")
	fmt.Println(mw.IdentifyImage())
	fmt.Println("8888888888888888888888888888888888888888888888888888888888888888888888")
	*/

	log.Println(">>> ResizeImage method starting.")
	log.Println(">>> Using given WIDTH=",i.width," as reference. Keeping aspect ratio...")

	// Uses current size to maintain aspect ratio of image. 
	// Uses the WIDTH in the REQUEST to find the correct height to keep correct image dimensions.
	iWidth := mw.GetImageWidth()
	iHeight := mw.GetImageHeight()

	// Resize image based on service query parameter = width
	wi, _ := strconv.Atoi(i.width)
	he := (uint(wi) * uint(iHeight))/uint(iWidth)

	err = mw.ResizeImage(uint(wi), uint(he), imagick.FILTER_BOX, 1)
	if err != nil {
		panic(err)
		return errors.New("Error resizing image.")
	}

	log.Println(">>> Setting compression...")
	err = mw.SetImageCompressionQuality(95)
	if err != nil {
		panic(err)
		return errors.New("Error setting compression quality.")
	}

	// Otherwise try WriteImage(string)
	log.Println(">>> Writting new resized image file.")
	err = mw.WriteImageFile(resizedFile)
	if err != nil {
		panic(err)
		return errors.New("Error writting into resized image file.")
	}

	fmt.Println("Resize complete!")
	return nil
}

// Moves resized image to the S3 Bucket.
func (i Image) uploadImage() error {
	fmt.Println("Starting uploade process...")

	creds := credentials.NewStaticCredentials(AccessKeyID, SecretAccessKey, "")

	_, err := creds.Get()
	if err != nil {
		panic(err)
		return errors.New("Error while getting credentials.")
	}

	aws.DefaultConfig.Region = "us-east-1" // Test with other regions.

	config := &aws.Config{
		Region: 			"",
		Endpoint:	 		"s3.amazonaws.com",
		S3ForcePathStyle:	true,
		Credentials: 		creds,
		LogLevel:			0,
	}

	s3client := s3.New(config)

	log.Println(">>>Opening file")
	file, err := os.Open(i.md5Key+"_r"+i.extension) // Maybe add complete path
	defer file.Close()

	if err != nil {
		panic(err)
		return errors.New("Error while opening file to upload.")
	}

	fileInfo, _ := file.Stat()
	var size int64 = fileInfo.Size()

	buffer := make([]byte, size)

	// Read file content to buffer
	file.Read(buffer)

	fileBytes := bytes.NewReader(buffer)
	fileType := http.DetectContentType(buffer)
	path := "/test_image_location/" + i.fileName

	params := &s3.PutObjectInput{
		Bucket: 		aws.String(Bucket), // Required
		Key: 			aws.String(path),	// Required
		ACL:           	aws.String("public-read"),
		Body: 			fileBytes,
		ContentLength: 	aws.Long(size),
		ContentType: 	aws.String(fileType),
		Metadata: 		map[string]*string{
			"Key": aws.String("MetadataValue"), // Required
			},
			// See more at http://godoc.org/github.com/aws/aws-sdk-go/service/s3#S3.PutObject
	}

	result, err := s3client.PutObject(params)

	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			// Generic AWS Error with Code, Mesage and original error (if any)
			fmt.Print(awsErr.Code(), awsErr.Message(), awsErr.OrigErr())
			if reqErr, ok := err.(awserr.RequestFailure); ok{
				// A service error ocurred
				fmt.Println(reqErr.Code(), reqErr.Message(), reqErr.StatusCode(), reqErr.RequestID())
			}
		} else {
			// This cae should never be hit, the SDK should always return an
			// Error which satisfies the awserr.Error interface.
			fmt.Println(err.Error())
		}
	}
	fmt.Println("Upload complete~")
	fmt.Println(awsutil.StringValue(result))
	return nil

}

// Deletes original image from local storage.
func (i Image) deleteImage() error {
	err := os.Remove(i.fileName)

	if err != nil {
		log.Println(err)
	}
	return err
} 

// Downloads image.
func (i Image) downloadImage() error {
	fmt.Println("Downloading image...")

	file, err := os.Create(i.fileName)

	if err != nil {
		log.Println(err)
		panic(err)
		return errors.New("Error creating file.")
	}

	defer file.Close()

	check := http.Client{
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			r.URL.Opaque = r.URL.Path
			return nil
		},
	}

	// Add a filter to check redirect
	resp, err := check.Get(i.url)

	if err != nil {
		log.Println(err)
		panic(err)
		return errors.New("Error while handling redirect.")
	}
	defer resp.Body.Close()
	log.Println(resp.Status)

	size, err := io.Copy(file, resp.Body)

	if err != nil {
		panic(err)
		return errors.New("Error writting file.")
	}

	log.Println(">>>>Size", size)
	log.Println(">>>>Download completed.", i.fileName)
	return nil
}	

// Checks if image has been processed already. Returns request data and boolean with the result(True/False)
func isImageProcessed(img string, w string, h string) (Image, bool){
	fmt.Println("Looking up image...", img+w+h)

	image := Image{
		url: img,
		extension: getImageExtension(img),
		width: w,
		height: h,
		md5Key: md5Encode(img, w, h),
		fileName: md5Encode(img, w, h) + getImageExtension(img),
		}  
	// TODO: search md5Key somewhere, i.e, Aerospike, database, etc
	// Currently always set to false, so we always process the image.
	found := false

	// Local image check...
	_, err := os.Stat(image.md5Key+"_r"+image.extension) 
	if err == nil {
    fmt.Printf("file exists; " + image.md5Key+"_r"+image.extension )
    	found = true
	} else {
		found = false
	}
	
	return image, found
}

// Checks correct request format
func requestHasCorrectFormat(img_url string, w string, h string) error{
	if len(img_url) > 0 && len(w) > 0 && len(h) > 0 {
		return nil
	} else {
		return errors.New("Invalid format. Missing mandatory parameter. Image URL, width and height are required.")
	}
}

// Encode map. Returns md5 already converted to String.
func md5Encode(img_url string, w string, h string) string {
	// String containing image_url+width+height (nospaces)
	md5Hash := md5.New()
	md5Hash.Write([]byte(img_url+w+h))
	return hex.EncodeToString(md5Hash.Sum(nil))
}

// Returns image extension as string.
func getImageExtension(img_url string) string{
	return filepath.Ext(img_url)
}

// Go routine-- Pool Worker
func poolWorker(id int, jobData <-chan Image) {
	fmt.Println("Starting pool worker>>>")
	for j := range jobData {
		log.Println(">>>	Resize job received! ")
		log.Println("Worker: ", id, "Processing: ", j)
		
		err := j.downloadImage()
		if err != nil {
			panic(err)
		}
		
		err = j.resizeImage()

		//TODO: fix uploader
		
		err = j.uploadImage()
		if err != nil {
			panic(err)
		}


		log.Println(">>DELETING ORIGINAL IMAGE....")
		err = j.deleteImage()

		if err != nil {
			panic(err)
		}
		fmt.Println("DELETED:", j.fileName)
		
		
		// TIP: Use channel to wait for process to finish?
		// TODO: Implement a wait.
		
		time.Sleep(time.Second) // Remove this after
		log.Println(">>>>	Job done!")
	}
}

func startIRService(processing chan Image) http.Handler {
	IRService := func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Starting image resize service...")

		err := requestHasCorrectFormat(r.FormValue("image_url"), r.FormValue("width"), r.FormValue("height"))
		if err != nil {
			panic(err)
		}

		/* 
		MD5_key is used to look-up/search previously processed images.
		This might be stored in Aerospike, database or other destination. If image not found, MD5_key or 
		parsed data from it is sent to buffered channel connected to pool of workers. 
		*/

		// IMPROVE: add err to destination variables to ensure everything went well?
		imageData, result := isImageProcessed(r.FormValue("image_url"), r.FormValue("width"), r.FormValue("height"))
		// Result tells us if image has been processed before.
		if result {
			// We want to return Amazon S3 image URL and use it for redirect.

			// Sends data of the image to returning channel
			// Image saved in S3 server with the md5Key+ext as image name
			log.Println("FOUND!")
			// TODO: redirect <- Amazon S3 server image URL.
			http.Redirect(w, r, "https://bucket-name/path/test_image_location/" + imageData.fileName, 302)
			
		} else {
			// Add imageData to worker queue
			log.Println("NOT FOUND!")
			// Sends imageData to processing channel
			processing <- imageData
			http.Redirect(w, r, imageData.url, 302)

		}
		
		/*
		Ideally, we want a single buffered channel(~120)[resizeChannel] receiving all
		MD5_keys that still need processing, this channel will feed a pool of working that will 
		performed the image processing in parallel.
		*/

		// Each worker will use one MD5_key to process and send a 'Done' signal back.
	}
	return http.HandlerFunc(IRService)
}
