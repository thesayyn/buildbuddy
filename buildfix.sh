#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"

gazelle=0
stage=0
commit=0
push=0
while [[ "$#" -gt 0 ]]; do
  case "$1" in
  -g | --gazelle)
    gazelle=1
    ;;
  -s | --stage)
    stage=1
    ;;
  -c | --commit)
    stage=1
    commit=1
    ;;
  -p | --push)
    stage=1
    commit=1
    push=1
    ;;
  *)
    echo "Usage: $0 [-g] [-s | -c | -p]"
    echo ""
    echo "Fix options:"
    echo "  -g, --gazelle  Run gazelle"
    echo ""
    echo "Git options:"
    echo "  -s, --stage    Interactively stage files ('git add') after formatting."
    echo "  -c, --commit   Commit staged files. Has no effect if there are existing staged changes. Implies -s."
    echo "  -p, --push     Push any commits that are made. Implies -c."
    exit 1
    ;;
  esac

  shift
done

c_yellow="\x1b[33m"
c_reset="\x1b[0m"

modified_before=$(mktemp)
modified_after=$(mktemp)
cleanup() {
  rm "$modified_before" "$modified_after"
  trap cleanup EXIT
}

git diff --name-only | sort >"$modified_before"

# buildifier format all BUILD files
echo "Formatting WORKSPACE/BUILD files..."
buildifier -r .

echo "Formatting .go files..."
# go fmt all .go files
gofmt -w .

if which clang-format &>/dev/null; then
  echo "Formatting .proto files..."
  git ls-files --exclude-standard | grep '\.proto$' | xargs -d '\n' --no-run-if-empty clang-format -i --style=Google
else
  echo -e "${c_yellow}WARNING: Missing clang-format tool; will not format proto files.${c_reset}"
fi

echo "Formatting frontend and markup files..."
bazel run --ui_event_filters=-info,-stdout,-stderr --noshow_progress //tools/prettier:fix

if ((gazelle)); then
  echo "Fixing BUILD dependencies with gazelle..."
  bazel run --ui_event_filters=-info,-stdout,-stderr --noshow_progress //:gazelle
fi

git diff --name-only | sort >"$modified_after"

echo 'All done!'

if ! ((stage)); then
  exit
fi
if ! [ -t 0 ] || ! [ -t 1 ]; then
  echo -e "${c_yellow}WARNING: not connected to a terminal; skipping interactive staging.${c_reset}"
  exit 1
fi

export PATH="$PATH:$HOME/go/bin"
if ! which fzf &>/dev/null; then
  echo -e "${c_yellow}WARNING: Missing fzf tool; did not stage any changes.${c_reset}"
  exit 1
fi

# Auto-commit only if there are no files staged before
# selecting files to add.
if ((commit)); then
  commit=$(git diff --staged --quiet && echo '1' || echo '0')
fi

diff -Pdpru "$modified_before" "$modified_after" | perl -n -e '/^\+([^+].*)/ && print "./$1\n"' |
  fzf --exit-0 --prompt='Stage modified files? Select with Arrows, Tab, Shift+Tab, Enter; quit with Ctrl+C > ' --multi |
  xargs -d '\n' --no-run-if-empty git add

if ((commit)) && ! git diff --staged --quiet; then
  git commit -m "Fix formatting / build issues"
  if ((push)); then
    git push
  fi
fi
