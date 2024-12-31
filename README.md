# go-cs2-cdn
This project retrieves the latest CS2 item images and publishes them for easy use with jsDelivr. 

The build process is automated to run every 24 hours, check for changes in the specific VPKs that contain the images, download, extract and decompile into the static directory from which the links are generated for use with jsDelivr. 

## Tools required
1. DepotDownloader
2. ValveResourceFormat 
