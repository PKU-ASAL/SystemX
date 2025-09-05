curl -X POST http://localhost:8080/api/v1/collectors/register \
>   -H "Content-Type: application/json" \
>   -d '{
>     "hostname": "remote-server-155",
>     "ip_address": "155.94.153.45",
>     "os_type": "linux",
>     "os_version": "3.10.0-693.11.6.el7.x86_64",
>     "deployment_type": "agentless",
>     "description": "Remote server for testing - 155.94.153.45"
>   }'
{"success":true,"data":{"collector_id":"b1de298c-38bd-479d-be94-459778086446","worker_url":"http://49.232.13.155:6000","script_download_url":"/api/v1/scripts/setup-terminal.sh?collector_id=b1de298c-38bd-479d-be94-459778086446"}}