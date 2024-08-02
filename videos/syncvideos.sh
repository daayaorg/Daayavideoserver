for ii in /Users/jp/git/daayavideoserver/videos/video?
do
    echo "Syncing $ii"
    rsync -avPz -e 'ssh  -i ~/Downloads/vserver_key_azure_nirmit.pem'  $ii  azureuser@48.217.169.49:/var/daaya/videos
done


