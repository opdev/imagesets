Hive requires an ImageSet which points to an image for running an OpenShift installation  
via the [installer](https://github.com/openshift/installer).  

This binary pulls the latest x86_64 image references from quay.io, stores them in a csv,  
pushes the contents to a Google Sheet, creates an ImageSet in the Hive namespace for each,  
and populates the Google Form for lab requests with the image name.

Building quickly on local machine:  
docker run --rm -v "$PWD":/usr/src/imagesets -w /usr/src/imagesets golang:alpine go build -o build/imagesets -v

A Dockerfile exists in build directory:
cd build  
docker build -t {registry_url}:{registry_port}/{namespace}/imagesets:0.0.1 .  
