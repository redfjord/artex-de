#!/bin/sh
# ARTEX supervisor startup script (Linux / macOS / Docker ENTRYPOINT)
#
# Usage:
#   ./start.sh                        Run in the foreground (Ctrl-C to stop)
#   nohup ./start.sh >artex.log 2>&1 &    Run in the background
#   ./start.sh -addr :9000            Extra arguments are passed through to artex
#
# It does one thing only: run artex, then decide from the exit code whether to
# start it again.
#
#   0      User stopped normally  → exit the loop
#   75     Program requested restart → rerun immediately (page clicked "Update" or "Rollback")
#   other  Crash                  → back off and rerun (1→2→4… up to 60 seconds)
#
# Deliberately no download, SHA256 verification, or binary swap here: those
# would have to be written twice (sh and bat), and they are exactly the step
# that must never go wrong—if a broken binary were swapped in, this script would
# faithfully keep restarting it and the user would have to rescue the machine by
# hand. So verification/swapping all stays in Go (the selfupdate package), done
# by artex itself at startup; the script stays simple.
set -u

cd "$(dirname "$0")" || exit 1

BIN=./artex
[ -x "$BIN" ] || { echo "[artex] executable not found: $BIN" >&2; exit 1; }

RESTART_CODE=75
MAX_DELAY=60

child=0
stopping=0

# Forward the stop signal to artex itself.
#
# This is required under Docker: docker stop only sends SIGTERM to PID 1 (this
# script), not to the child process. Without forwarding, artex never receives
# the signal and cannot shut down gracefully; it is SIGKILLed after 10 seconds
# and running tasks are cut off mid-flight.
forward() {
	stopping=1
	if [ "$child" -ne 0 ]; then
		kill -TERM "$child" 2>/dev/null || true
	fi
}
trap forward INT TERM

# Wait for either the application or its restart delay. A stop that arrived
# between starting the child and recording its PID must reach that child too.
wait_for_child() {
	if [ "$stopping" -eq 1 ]; then
		kill -TERM "$child" 2>/dev/null || true
	fi
	wait "$child"
	code=$?
	if [ "$code" -gt 128 ]; then
		wait "$child"
		code=$?
	fi
	child=0
}

delay=1
while [ "$stopping" -eq 0 ]; do
	"$BIN" "$@" &
	child=$!

	# The signal interrupts wait and makes it return >128. The child is still
	# shutting down gracefully at that point, so wait once more to get its real
	# exit code.
	wait_for_child

	if [ "$stopping" -eq 1 ]; then
		echo "[artex] stopped"
		exit 0
	fi

	case "$code" in
		0)
			echo "[artex] exited normally"
			exit 0
			;;
		"$RESTART_CODE")
			# Update/rollback is ready: on rerun artex completes the swap at startup (see selfupdate.Bootstrap).
			echo "[artex] restart requested (applying new version)…"
			delay=1
			;;
		*)
			echo "[artex] exited abnormally (code=$code), restarting in ${delay}s" >&2
			sleep "$delay" &
			child=$!
			wait_for_child
			delay=$((delay * 2))
			[ "$delay" -gt "$MAX_DELAY" ] && delay=$MAX_DELAY
			;;
	esac
done
echo "[artex] stopped"
exit 0