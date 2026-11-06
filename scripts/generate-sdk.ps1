# 从唯一协议源生成 SDK；固定工具版本以保证可重复生成。
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$versions = @{
    'protoc' = '^libprotoc 33\.0$'
    'protoc-gen-go' = '^protoc-gen-go(?:\.exe)? v1\.36\.12$'
    'protoc-gen-go-grpc' = '^protoc-gen-go-grpc 1\.6\.0$'
}
foreach ($tool in $versions.Keys) {
    $actual = & $tool --version
    if ($LASTEXITCODE -ne 0 -or $actual -notmatch $versions[$tool]) {
        throw "工具版本不匹配：$tool，实际为 $actual"
    }
}
Push-Location $repoRoot
try {
    # 兼容 Windows PowerShell 5.1：不用 .NET Core 才提供的 IO.Path.GetRelativePath
    $prefixLength = $repoRoot.TrimEnd('\', '/').Length + 1
    $protoFiles = @(Get-ChildItem api -Recurse -Filter *.proto | Sort-Object FullName | ForEach-Object {
        $_.FullName.Substring($prefixLength).Replace('\', '/')
    })
    if ($protoFiles.Count -eq 0) { throw 'api/ 下没有协议文件' }
    $module = 'serv_tpl/sdk'
    & protoc --proto_path=. --go_out=sdk "--go_opt=module=$module" --go-grpc_out=sdk "--go-grpc_opt=module=$module" @protoFiles
    if ($LASTEXITCODE -ne 0) { throw 'SDK 生成失败' }
} finally {
    Pop-Location
}
