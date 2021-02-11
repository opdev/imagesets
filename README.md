Hive requires an ImageSet which points to an image for running an OpenShift installation  
via the [installer](https://github.com/openshift/installer).  

This binary pulls the latest x86_64 image references from quay.io, stores them in a csv,  
pushes the contents to a Google Sheet, creates an ImageSet in the Hive namespace for each,  
and populates the Google Form for lab requests with the image name.
