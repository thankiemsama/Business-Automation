param([string]$BaseUrl = "http://127.0.0.1:8080")
$ErrorActionPreference = "Stop"
$api = "$BaseUrl/api/v1"
$headers = @{}
function Call-Api([string]$Method, [string]$Path, $Body) {
    $argsForRequest = @{ Method=$Method; Uri="$api$Path"; Headers=$headers; ContentType="application/json; charset=utf-8" }
    if ($null -ne $Body) { $argsForRequest.Body = [System.Text.Encoding]::UTF8.GetBytes(($Body | ConvertTo-Json -Depth 10)) }
    (Invoke-RestMethod @argsForRequest).data
}
$email = "demo-$([guid]::NewGuid().ToString('N'))@example.com"
$password = [guid]::NewGuid().ToString('N') + '!Aa9'
$null = Call-Api POST '/auth/register' @{name='Demo Business';email=$email;password=$password}
$login = Call-Api POST '/auth/login' @{email=$email;password=$password}
$headers.Authorization = "Bearer $($login.access_token)"
$customer = Call-Api POST '/customers' @{name='Khách hàng demo';email='customer@example.com'}
$null = Call-Api POST '/knowledge' @{subject='Chính sách hoàn tiền';content='Doanh nghiệp tiếp nhận yêu cầu hoàn tiền trong 7 ngày.';enabled=$true}
$null = Call-Api POST '/automations' @{name='Xử lý yêu cầu hoàn tiền';keywords=@('hoàn tiền');enabled=$true}
$conversation = Call-Api POST '/conversations' @{customer_id=$customer.id;channel='web'}
$null = Call-Api POST "/conversations/$($conversation.id)/messages" @{sender='customer';content='Tôi muốn hỏi chính sách hoàn tiền'}
$reply = Call-Api POST "/conversations/$($conversation.id)/reply" $null
$dashboard = Call-Api GET '/dashboard' $null
$reply | ConvertTo-Json -Depth 10
$dashboard | ConvertTo-Json -Depth 10
$null = Call-Api POST '/auth/logout' $null
