BASE=`dirname $0`

oc create secret generic connect-config --from-file=$BASE/connect.json
oc create -f $BASE/responder-a.yaml
oc create -f $BASE/responder-b.yaml
while [ $(oc get pod responder-a -o jsonpath='{.status.phase}') != 'Running' ] || [ $(oc get pod responder-b -o jsonpath='{.status.phase}') != 'Running' ]; do
  echo "Waiting for responders..."; sleep 1
done
oc create -f $BASE/sender.yaml
while [ $(oc get pod sender -o jsonpath='{.status.phase}') != 'Succeeded' ]; do
  echo "Waiting for sender..."; sleep 1
done
echo "Test completed"
oc logs sender

echo "Cleaning up"
$BASE/cleanup.sh
