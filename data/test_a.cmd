@echo off
rem A: lockout reason check
echo registering crlk...
curl -s -o NUL -w "reg=%{http_code}\n" -X POST http://127.0.0.1:8082/api/v1/auth/register -H "Content-Type: application/json" -H "X-Nexa-Client-IP: 10.6.6.1" -H "X-Nexa-Proxy-Secret: e2e2-proxy-secret-abcdef0123456789" -d "{\"name\":\"Crlk\",\"email\":\"crlk@e2e2.test\",\"password\":\"password-123456\"}"
echo 5 wrong attempts...
for /L %%i in (1,1,5) do (
  curl -s -o NUL -X POST http://127.0.0.1:8082/api/v1/auth/login -H "Content-Type: application/json" -H "X-Nexa-Client-IP: 10.6.6.2" -H "X-Nexa-Proxy-Secret: e2e2-proxy-secret-abcdef0123456789" -d "{\"email\":\"crlk@e2e2.test\",\"password\":\"wrong-wrong-1234\"}"
)
echo correct attempt while locked...
curl -s -X POST http://127.0.0.1:8082/api/v1/auth/login -H "Content-Type: application/json" -H "X-Nexa-Client-IP: 10.6.6.2" -H "X-Nexa-Proxy-Secret: e2e2-proxy-secret-abcdef0123456789" -d "{\"email\":\"crlk@e2e2.test\",\"password\":\"password-123456\"}"
echo.
echo === B-prep: register exp user + forgot ===
curl -s -o NUL -w "reg_exp=%{http_code}\n" -X POST http://127.0.0.1:8082/api/v1/auth/register -H "Content-Type: application/json" -H "X-Nexa-Client-IP: 10.4.4.1" -H "X-Nexa-Proxy-Secret: e2e2-proxy-secret-abcdef0123456789" -d "{\"name\":\"Exp\",\"email\":\"exp@e2e2.test\",\"password\":\"password-123456\"}"
curl -s -w "\nforgot=%{http_code}\n" -X POST http://127.0.0.1:8082/api/v1/auth/forgot -H "Content-Type: application/json" -H "X-Nexa-Client-IP: 10.4.4.1" -H "X-Nexa-Proxy-Secret: e2e2-proxy-secret-abcdef0123456789" -d "{\"email\":\"exp@e2e2.test\"}"
echo === C: invite flow ===
curl -s -o NUL -w "reg_team=%{http_code}\n" -X POST http://127.0.0.1:8082/api/v1/auth/register -H "Content-Type: application/json" -H "X-Nexa-Client-IP: 10.5.5.1" -H "X-Nexa-Proxy-Secret: e2e2-proxy-secret-abcdef0123456789" -d "{\"name\":\"Inv Owner\",\"email\":\"invowner@e2e2.test\",\"password\":\"password-123456\"}"
echo done
