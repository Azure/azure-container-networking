### Running the dropgz locally

Select the file(for example azure-ipam binary) you want to deploy using the dropgz.

1. Copy the file (i.e azure-ipam) to the directory `/dropgz/pkg/embed/fs` 
2. Add the sha of the file to the sum.txt file.(`sha256sum * > sum.txt`)
3. You need to gzip the file, so run the cmd `gzip --verbose --best --recursive azure-ipam` and rename the output .gz file to original file name.
4. Do the step 3 for `sum.txt` file as well.
5. go to dropgz directory and build it. (`go build .`)
6. You can now test the dropgz command locally. (`./dropgz deploy azure-ipam -o ./azure-ipam`)