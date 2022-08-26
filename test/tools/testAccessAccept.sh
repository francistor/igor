# --------------------------------------------------------------
# Simple Access-Accept test for Radius
# --------------------------------------------------------------

export _THIS_FILE_DIRNAME=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source $_THIS_FILE_DIRNAME/env.rc

# Test parameters
REQUESTFILE=$_THIS_FILE_DIRNAME/AccessRequest.txt

COUNT=1

# Delete Garbage
rm $_THIS_FILE_DIRNAME/out/*

# Diameter CCR -------------------------------------------------------------
echo 
echo Access-Request
echo

# echo User-Name = \"test@accept\" > $REQUESTFILE
# echo User-Password = \"hi, this is the __ password!\" > $REQUESTFILE
# echo Tunnel-Type = \"PPTP\":2 > $REQUESTFILE
# echo Igor-IntegerAttribute = 1 > $REQUESTFILE
# echo 3GPP2-Pre-Shared-Secret = \"000000\" > $REQUESTFILE
# echo 3GPP2-MN-HA-Key = \"000000\" >> $REQUESTFILE
# echo Igor-SaltedOctetsAttribute = \"0\" > $REQUESTFILE
# echo Tunnel-Client-Endpoint= \"t\":2 >> $REQUESTFILE
echo Tunnel-Password = \"1\":2 > $REQUESTFILE

# Send the packet
$RADIUS -logLevel Verbose -retryCount 1 -count $COUNT -remoteAddress 127.0.0.1:1812 -code Access-Request -request "@$REQUESTFILE"