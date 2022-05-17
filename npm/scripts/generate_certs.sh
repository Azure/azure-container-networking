#!/bin/bash

CERTS_STAGING_DIR=.
SAN_CNF_FILE=san.cnf
CERTIFICATE_VALIDITY_DAYS=3650
CERT_SUBJ="/C=US/ST=Washington/L=Redmond/O=Microsoft/OU=Azure/CN=azure-npm.kube-system.svc.cluster.local"

# Check if openssl is installed
if ! command -v openssl &> /dev/null
then
  echo "openssl could not be found"
  exit
fi

# Check if SAN_CNF_FILE exists
if [ ! -f "$SAN_CNF_FILE" ]
then
  echo "SAN_CNF_FILE does not exist"
  exit
fi

if [ ! -d "$CERTS_STAGING_DIR" ]
then
  echo "Creating $CERTS_STAGING_DIR"
  mkdir -p $CERTS_STAGING_DIR
fi

# Generate the ca certificate and key
openssl req -x509 -newkey rsa:4096 -days $CERTIFICATE_VALIDITY_DAYS -nodes -keyout $CERTS_STAGING_DIR/ca.key -out $CERTS_STAGING_DIR/ca.crt -subj $CERT_SUBJ

# Create a certificate signing request for the server
openssl req -newkey rsa:4096 -nodes -keyout $CERTS_STAGING_DIR/tls.key -out $CERTS_STAGING_DIR/server-req.pem -config $SAN_CNF_FILE -extensions v3_req -subj $CERT_SUBJ

# Sign the server certificate with the CA
openssl x509 -req -in $CERTS_STAGING_DIR/server-req.pem -CA $CERTS_STAGING_DIR/ca.crt -CAkey $CERTS_STAGING_DIR/ca.key -CAcreateserial -out $CERTS_STAGING_DIR/tls.crt --days $CERTIFICATE_VALIDITY_DAYS --extfile $SAN_CNF_FILE --extensions v3_req

# Remove the secret CA key and signing request
rm -rf $CERTS_STAGING_DIR/ca.key $CERTS_STAGING_DIR/server-req.pem
