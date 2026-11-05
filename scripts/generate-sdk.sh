#!/usr/bin/env bash
# 从唯一协议源生成 SDK；固定工具版本以保证可重复生成。
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

check_version() {
    local tool="$1" expected="$2" actual
    actual="$("$tool" --version)"
    actual="${actual%$'\r'}"
    if [[ ! "$actual" =~ $expected ]]; then
        printf '工具版本不匹配：%s，实际为 %s\n' "$tool" "$actual" >&2
        exit 1
    fi
}

check_version protoc '^libprotoc 33\.0$'
check_version protoc-gen-go '^protoc-gen-go(\.exe)? v1\.36\.12$'
check_version protoc-gen-go-grpc '^protoc-gen-go-grpc 1\.6\.0$'

cd -- "$repo_root"
# 用数组和 NUL 分隔读取路径，避免空格被拆成多个 protoc 参数。
proto_files=()
while IFS= read -r -d '' proto; do
    proto_files+=("$proto")
done < <(find api -type f -name '*.proto' -print0)
if (( ${#proto_files[@]} == 0 )); then
    printf 'api/ 下没有协议文件\n' >&2
    exit 1
fi

module='gitee.com/chengduzhihuige/go-serv-template/sdk'
protoc --proto_path=. --go_out=sdk "--go_opt=module=$module" \
    --go-grpc_out=sdk "--go-grpc_opt=module=$module" "${proto_files[@]}"
