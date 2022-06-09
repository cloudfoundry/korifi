RED=1
GREEN=2
BLUE=4

print_message() {
  message=$1
  colour=$2
  printf "\\r\\033[00;3%sm%s\\033[0m\\n" "$colour" "$message"
}

warning=$(
  cat <<EOF
** WARNING **

This an example script used to manage a standalone Eirini-Controller deployment.
It is used internally for testing, but is not supported for external use.

EOF
)

print_disclaimer() {
  print_message "$warning" "$BLUE"
}
