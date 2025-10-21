if [[ -z "${env:$GSU_BUCKET_URL}" ]]; then
  echo 'No Signing Bucket URL, skipping signing'
  exit 0
fi

file_filter=(
"out/bin/fluent-bit.exe"
"out/bin/fluent-bit.dll"
"out/bin/google-cloud-metrics-agent.exe"
"out/bin/google-cloud-ops-agent.exe"
"out/bin/google-cloud-ops-agent-diagnostics.exe"
"out/bin/google-cloud-ops-agent-wrapper.exe"
"pkg/goo/maint.ps1"
)

for file in "${file_filter[@]}"
do
  echo "Sending: $file to be signed"
  gsutil cp "./$file" "${env:GSU_BUCKET_URL}$file"
done

# Sent to indicate all binaries have been sent to be signed
touch ./UNSIGNED_READY.txt

gsutil cp ./UNSIGNED_READY.txt "${env:GSU_BUCKET_URL}UNSIGNED_READY.txt"

echo "Sent all unsigned binaries"

# Wait for binaries to be signed.
i=0
until gsutil -q stat "${env:GSU_BUCKET_URL}SIGNED_READY.txt"; do
  if [[ $i -ge 300 ]]; then
    echo "Could not get signed binaries"
    exit 1
  elif [[ $i -gt 0 ]]; then
    # Sleep for 15 seconds before the next attempt to avoid hammering the timestamp server.
    sleep 15
  fi
  i=$((i+1))
done

for file in "${file_filter[@]}"
do
  echo "Receiving: signed $file"
  gsutil cp "${env:GSU_BUCKET_URL}$file" "./$file"
done

gsutil rm "${env:GSU_BUCKET_URL}*.goo"

powershell -c & .\\pkg\\goo\\build.ps1 -DestDir /work/out;
