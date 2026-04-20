pkill -f "http.server 5175"  &&  nohup python3 -m http.server 5175 > /tmp/mockups.log 2>&1 &
