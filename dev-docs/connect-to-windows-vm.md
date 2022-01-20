# Connect to a Windows VM

Warning: Connection to Windows VMs may require a VPN. Follow instruction to
set it up, and make sure you are on VPN before proceeding to the following step.

Note: The official GCP instruction to connect to a Windows GCE VM is
https://cloud.google.com/compute/docs/instances/connecting-to-windows. There are various
ways of doing it. `Chrome RDP plugin` is simpler to set up, but it has poor
performance. And the UI windows scrolling is not very user friendly. This may
fit in a use case where you just want to spot check something simple. `Microsoft
Remote Desktop (Mac)` takes longer for the initial setup, but is recommended for
non-trivial interactions with Windows VMs.

## Microsoft Remote Desktop (Mac)

<details>
<summary>One-time setup on Mac</summary>

*   Install `gcloud` following https://cloud.google.com/sdk/docs/quickstart#mac.
*   Install `Microsoft Remote Desktop for Mac`
*   Create a PC in `Microsoft Remote Desktop for Mac` that connects to
    `localhost:13000`.
    ![image](https://user-images.githubusercontent.com/5287526/133005613-eab25386-fcda-4554-a156-6628803390c0.png)

*   Set up [IAP](https://cloud.google.com/iap/docs/using-tcp-forwarding)
    permission by running the following command:

    ```shell
    $ gcloud projects add-iam-policy-binding ${USER}-sandbox \
        --member=user:${USER}@google.com \
        --role=roles/iap.tunnelResourceAccessor
    ```

</details>

WARNING: Make sure you have finished the `One-time setup on Mac` section before
proceeding.

**Connect to a specific VM**

1.  Start an IAP tunnel to the VM.

    Customize `$VM_PROJECT_ID`, `$VM_ZONE`, and `$IAP_VM_NAME` as needed.

    ```shell
    export VM_PROJECT_ID=${USER}-sandbox
    export VM_ZONE=us-central1-a
    export IAP_VM_NAME=${USER}-win-build-vm

    gcloud compute firewall-rules create allow-rdp-ingress-from-iap \
        --direction=INGRESS \
        --action=allow \
        --rules=tcp:3389 \
        --source-ranges=35.235.240.0/20

    gcloud compute start-iap-tunnel \
        --project $VM_PROJECT_ID --zone $VM_ZONE \
        --local-host-port=localhost:13000 \
        $IAP_VM_NAME 3389
    ```

1.  Open Microsoft Remote Desktop app, double click on the `localhost:13000` PC
    to connect to it. Use the password you recorded in the previous
    `gcloud compute reset-windows-password` step when
    prompted to do so.

   ![image](https://user-images.githubusercontent.com/5287526/133006175-a1f2b019-1a75-4f62-9c3c-18759f951d90.png)


<details>
<summary>Troubleshooting</summary>

If you run into an error like below, it's probably because the previously
created firewall rule has been wiped. Either have your
project exempted or re-create the firewall rule.

```
Testing if tunnel connection works.
ERROR: (gcloud.compute.start-iap-tunnel) Error while connecting [4003: 'failed to connect to backend']."
```

or

```
ERROR: Error while receiving from client.
Traceback (most recent call last):
  File "/Applications/google-cloud-sdk/lib/googlecloudsdk/command_lib/compute/iap_tunnel.py", line 573, in _HandleNewConnection
    self._RunReceiveLocalData(conn, repr(socket_address))
  File "/Applications/google-cloud-sdk/lib/googlecloudsdk/command_lib/compute/iap_tunnel.py", line 460, in _RunReceiveLocalData
    store.LoadIfEnabled(use_google_auth=True)))
  File "/Applications/google-cloud-sdk/lib/googlecloudsdk/command_lib/compute/iap_tunnel.py", line 431, in _InitiateWebSocketConnection
    new_websocket.InitiateConnection()
  File "/Applications/google-cloud-sdk/lib/googlecloudsdk/api_lib/compute/iap_tunnel_websocket.py", line 131, in InitiateConnection
    self._WaitForOpenOrRaiseError()
  File "/Applications/google-cloud-sdk/lib/googlecloudsdk/api_lib/compute/iap_tunnel_websocket.py", line 336, in _WaitForOpenOrRaiseError
    raise ConnectionCreationError(error_msg)
googlecloudsdk.api_lib.compute.iap_tunnel_websocket.ConnectionCreationError: Error while connecting [4003: 'failed to connect to backend'].
```

</details>

## Chrome RDP plugin

**One-time setup in Chrome**

*   Install the
    [Chrome RDP Plugin](https://chrome.google.com/webstore/detail/mpbbnannobiobpnfblimoapbephgifkm).

**Connect to a specific VM**

1.  Go to the GCE VM page at
    https://console.cloud.google.com/compute/instances?project=$VM_PROJECT_ID,
    find the target VM and click `RDP`.
    
    ![image](https://user-images.githubusercontent.com/5287526/133006249-97aa3501-dad0-4af3-936e-983ebac70bc2.png)


1.  In the popped-up Chrome RDP plugin window there should be a prompt for you
    to enter credentials. Put in the password retrieved from the previous
    `gcloud compute reset-windows-password` step and hit OK.
    
    ![image](https://user-images.githubusercontent.com/5287526/133006296-04c4a372-e93b-4aac-b578-02a2e2d540ad.png)

</details>
