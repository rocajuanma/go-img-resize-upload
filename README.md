# go-img-resize-upload
Golang HTTP Service to download an image, resize it and upload it to an S3 Amazon Bucket. Possibility to redirect to newly resized and uploaded image. A work in progress.

# Usage
1. Replace the S3 Bucket information
2. Run the go file: `go run imageResizeUploadS3.go`
3. In your browser, hit the `/r/` handler from your localhost. Inside `imageResizeUploadS3.go`, you can specify the port to be used. It is currently configured to use port: 3000. You must provide the correct parameters in the request: image_url, width, height.
    For instance:
      `localhost:3000/r?image_url=https://pbs.twimg.com/profile_images/604644048/sign051.gif&width=200&height=200`

Image specified in the request will be downloaded and resized. Currently, the actual image url is used as a redirect because the logic follows this flow:
  If image does not exist:
    Redirect to actual image, download and resize. New resized image will be uploadd to S3 to a unique location(md5 + dimensions). This is done so that the next time this image is included in the request, we will look up the stored image.
  If image does exist:
    Redirect to the previously resized image stored in the S3 Bucket.
