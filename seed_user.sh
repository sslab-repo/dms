ADMIN_USERNAME="sslab123"
read -s -p "Admin password: " ADMIN_PASSWORD
echo

TEST_USERNAME="dataset-test"
TEST_PASSWORD="$(openssl rand -hex 8)"

TOKEN=$(curl -sS -X POST http://localhost:8081/api/auth/login \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$ADMIN_USERNAME\",\"password\":\"$ADMIN_PASSWORD\"}" \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])')

curl -sS -X POST http://localhost:8081/api/admin/users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"username\": \"$TEST_USERNAME\",
    \"display_name\": \"Dataset Test User\",
    \"password\": \"$TEST_PASSWORD\",
    \"role\": \"researcher\"
  }"

echo
echo "Test account created:"
echo "Username: $TEST_USERNAME"
echo "Password: $TEST_PASSWORD"