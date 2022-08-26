
# -------------------------------------------------------------
# Simple diameter send command using NokiaAAA
# --------------------------------------------------------------
export _THIS_FILE_DIRNAME=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source _THIS_FILE_DIRNAME/env.rc

ORIGIN_HOST=nokiaaaa.nokia
ORIGIN_REALM=nokia
APPLICATION_ID=Credit-Control
COMMAND=Credit-Control
DESTINATION_HOST=server.igorserver
DESTINATION_REALM=igorserver
DESTINATION_ADDRESS=127.0.0.1:3868

# Test parameters
REQUESTFILE=$_THIS_FILE_DIRNAME/CCRequest.txt

COUNT=1

# Delete Garbage
rm $_THIS_FILE_DIRNAME/out/*

# Diameter CCR -------------------------------------------------------------
echo 
echo Credit Control request
echo

echo Session-Id = \"session-id-1\" > $REQUESTFILE
echo Auth-Application-Id = 4 >> $REQUESTFILE
echo CC-Request-Type = 1 >> $REQUESTFILE
echo CC-Request-Number = 1 >> $REQUESTFILE
echo Subscription-Id = "Subscription-Id-Type=1, Subscription-Id-Data=913374871" >> $REQUESTFILE


# Send the packet
$DIAMETER -debug verbose -count $COUNT -oh $ORIGIN_HOST -or $ORIGIN_REALM -dh $DESTINATION_HOST -dr $DESTINATION_REALM -destinationAddress $DESTINATION_ADDRESS -Application $APPLICATION_ID -command $COMMAND -request "@$REQUESTFILE"