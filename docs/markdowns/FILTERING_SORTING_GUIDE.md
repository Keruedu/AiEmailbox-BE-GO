# Hướng dẫn chạy và kiểm tra Filtering & Sorting

## 1. Khởi động Backend

### Option 1: Sử dụng script có sẵn
```powershell
cd "d:\FIT@HCMUS\HK1_25-26\AWAD\Project\Sources\AiEmailbox-BE-GO"
.\run.ps1
```

### Option 2: Chạy trực tiếp
```powershell
cd "d:\FIT@HCMUS\HK1_25-26\AWAD\Project\Sources\AiEmailbox-BE-GO"
go run cmd/server/main.go
```

Backend sẽ chạy tại: http://localhost:8080

## 2. Khởi động Frontend

```powershell
cd "d:\FIT@HCMUS\HK1_25-26\AWAD\Project\Sources\AiEmailbox-FE"
npm run dev
```

Frontend sẽ chạy tại: http://localhost:3000

## 3. Kiểm tra API với Swagger UI

Truy cập: http://localhost:8080/swagger/index.html

Tại đây bạn có thể:
- Xem tất cả các API endpoints
- Test trực tiếp các API với query parameters mới:
  - `unread`: true/false - Lọc email chưa đọc
  - `hasAttachments`: true/false - Lọc email có đính kèm
  - `sortBy`: date/subject/sender - Sắp xếp theo trường
  - `sortOrder`: asc/desc - Thứ tự sắp xếp

## 4. Kiểm tra với HTTP Client (REST Client VSCode)

1. Cài đặt extension "REST Client" trong VSCode
2. Mở file: `examples/filtering_sorting.http`
3. Đăng nhập và lấy JWT token:
   - Gọi API login để lấy token
   - Copy token vào biến `@token` trong file
4. Click vào "Send Request" ở mỗi request để test

## 5. Test các tính năng

### Test Filter by Unread
```
GET /api/mailboxes/INBOX/emails?unread=true
```
Kết quả: Chỉ hiển thị các email chưa đọc

### Test Filter by Attachments
```
GET /api/mailboxes/INBOX/emails?hasAttachments=true
```
Kết quả: Chỉ hiển thị các email có đính kèm

### Test Sort by Date (Newest First)
```
GET /api/mailboxes/INBOX/emails?sortBy=date&sortOrder=desc
```
Kết quả: Emails được sắp xếp từ mới nhất đến cũ nhất

### Test Sort by Date (Oldest First)
```
GET /api/mailboxes/INBOX/emails?sortBy=date&sortOrder=asc
```
Kết quả: Emails được sắp xếp từ cũ nhất đến mới nhất

### Test Sort by Subject
```
GET /api/mailboxes/INBOX/emails?sortBy=subject&sortOrder=asc
```
Kết quả: Emails được sắp xếp theo tiêu đề A-Z

### Test Sort by Sender
```
GET /api/mailboxes/INBOX/emails?sortBy=sender&sortOrder=asc
```
Kết quả: Emails được sắp xếp theo email người gửi A-Z

### Test Combined Filters
```
GET /api/mailboxes/INBOX/emails?unread=true&hasAttachments=true&sortBy=date&sortOrder=desc
```
Kết quả: Chỉ hiển thị emails chưa đọc, có đính kèm, sắp xếp từ mới đến cũ

## 6. Kiểm tra từ Frontend

1. Đăng nhập vào ứng dụng
2. Vào trang Inbox
3. Sử dụng các bộ lọc và sắp xếp trên UI:
   - Toggle "Unread Only" 
   - Toggle "Has Attachments"
   - Chọn sort order từ dropdown

## 7. Debug và kiểm tra logs

### Backend logs
Backend sẽ in ra console khi nhận requests. Quan sát:
- Query parameters được gửi lên
- Kết quả trả về
- Có lỗi nào không

### Frontend logs
Mở DevTools (F12) và xem tab Console để kiểm tra:
- API calls được gửi với đúng parameters
- Response data có đúng như mong đợi

## 8. Xác minh kết quả

### Cách kiểm tra đúng sai:

1. **Filter by Unread:**
   - Check trường `isRead` trong response
   - Tất cả emails trả về phải có `isRead: false`

2. **Filter by Attachments:**
   - Check trường `hasAttachments` trong response
   - Tất cả emails trả về phải có `hasAttachments: true`
   - Kiểm tra `attachments` array không rỗng

3. **Sort by Date:**
   - Check trường `receivedAt` 
   - Với `sortOrder=desc`: dates giảm dần (mới → cũ)
   - Với `sortOrder=asc`: dates tăng dần (cũ → mới)

4. **Sort by Subject:**
   - Check trường `subject`
   - Với `sortOrder=asc`: A → Z
   - Với `sortOrder=desc`: Z → A

5. **Sort by Sender:**
   - Check trường `from.email`
   - Sắp xếp theo alphabet

## 9. Troubleshooting

### Backend không start được
```powershell
# Kiểm tra port 8080 có đang được sử dụng
netstat -ano | findstr :8080

# Build lại nếu cần
go build -o bin/server.exe cmd/server/main.go
```

### Gmail API không hoạt động
- Kiểm tra credentials.json
- Kiểm tra token.json
- Xem logs để biết lỗi cụ thể

### Không có emails trả về
- Đảm bảo đã sync emails từ Gmail
- Kiểm tra mailboxId có đúng không (INBOX, SENT, etc.)
- Thử bỏ filters để xem có emails không

### Filter không hoạt động như mong đợi
- Kiểm tra Gmail API query syntax
- Xem backend logs để biết query được gửi đến Gmail
- Thử test trực tiếp trên Gmail web với cùng query

## 10. API Reference

### Endpoint
```
GET /api/mailboxes/{mailboxId}/emails
```

### Query Parameters
| Parameter | Type | Description | Values | Default |
|-----------|------|-------------|--------|---------|
| page | int | Trang số | 1, 2, 3... | 1 |
| perPage | int | Số emails mỗi trang | 10, 20, 50... | 50 |
| unread | bool | Lọc email chưa đọc | true, false | false |
| hasAttachments | bool | Lọc email có đính kèm | true, false | false |
| sortBy | string | Trường sắp xếp | date, subject, sender | date |
| sortOrder | string | Thứ tự sắp xếp | asc, desc | desc |

### Response
```json
{
  "emails": [...],
  "total": 100,
  "page": 1,
  "perPage": 50,
  "hasNextPage": true
}
```

## 11. Ví dụ cURL

```bash
# Filter unread emails
curl -X GET "http://localhost:8080/api/mailboxes/INBOX/emails?unread=true" \
  -H "Authorization: Bearer YOUR_TOKEN"

# Sort by date (oldest first)
curl -X GET "http://localhost:8080/api/mailboxes/INBOX/emails?sortBy=date&sortOrder=asc" \
  -H "Authorization: Bearer YOUR_TOKEN"

# Combined filters
curl -X GET "http://localhost:8080/api/mailboxes/INBOX/emails?unread=true&hasAttachments=true&sortBy=date&sortOrder=desc" \
  -H "Authorization: Bearer YOUR_TOKEN"
```
