#!/bin/bash

set -euo pipefail
IFS=$'\n\t'

RED=1
GREEN=2
print_message() {
  message=$1
  colour=$2
  printf "\\r\\033[00;3%sm[%s]\\033[0m\\n" "$colour" "$message"
}

main() {
  print_message "about to run tests in parallel, it will be awesome" $GREEN
  print_message "ctrl-d panes when they are done" $RED
  tmux new-window -n korifi-tests "/bin/bash -c \"make lint && make -C tools test && make -C migration test ; bash --init-file <(echo 'history -s make lint \&\& make -C tools test \&\& make -C migration test ')\""
  tmux split-window -h -p 75 "GINKGO_NODES=2 /bin/bash -c \"make -C kpack-image-builder test; bash --init-file <(echo 'history -s make -C kpack-image-builder test')\""
  tmux split-window -h -p 67 "GINKGO_NODES=2 /bin/bash -c \"make -C statefulset-runner test; bash --init-file <(echo 'history -s make -C statefulset-runner test')\""
  tmux split-window -h -p 50 "GINKGO_NODES=2 /bin/bash -c \"make -C job-task-runner test; bash --init-file <(echo 'history -s make -C job-task-runner test')\""
  tmux split-window -vfb -p 66 "/bin/bash -c \"make -C api test && make -C controllers test; bash --init-file <(echo 'history -s make -C api test \&\& make -C controllers test')\""
  tmux split-window -h -p 50 "/bin/bash -c \"make test-e2e; bash --init-file <(echo 'history -s make test-e2e')\""
}

main
