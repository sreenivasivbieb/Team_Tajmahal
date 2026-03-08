# Codrix.ai — EC2 Backend Deployment Guide

## Prerequisites
- AWS EC2 instance (Ubuntu 22.04+ recommended, t3.medium or larger)
- Security group allowing inbound TCP on port **8080** (or 80/443 if using a reverse proxy)
- Docker & Docker Compose installed on the EC2 instance
- AWS credentials for Bedrock (IAM role or access keys)

## 1. Install Docker on EC2

```bash
# SSH into your EC2 instance
ssh -i your-key.pem ubuntu@<EC2_PUBLIC_IP>

# Install Docker
sudo apt update && sudo apt install -y docker.io docker-compose-plugin
sudo usermod -aG docker $USER
# Log out and back in for group changes
```

## 2. Clone the Repository

```bash
git clone https://github.com/sreenivasivbieb/Team_Tajmahal.git
cd Team_Tajmahal/vyuha-ai
```

## 3. Configure Environment

Create a `.env` file in the `vyuha-ai/` directory:

```bash
cat > .env << 'EOF'
AWS_ACCESS_KEY_ID=your-access-key
AWS_SECRET_ACCESS_KEY=your-secret-key
AWS_DEFAULT_REGION=us-east-1
EOF
```

> **Recommended**: Use an IAM instance role instead of access keys. If attached to the EC2 instance, remove the AWS_* variables from `.env`.

## 4. Build & Start the Backend

```bash
# Build and start using the backend-only Docker Compose
docker compose -f docker-compose.backend.yml up -d --build

# Check logs
docker compose -f docker-compose.backend.yml logs -f

# Verify health
curl http://localhost:8080/health
```

## 5. Verify the API

```bash
curl http://localhost:8080/health
# Expected: {"status":"ok"}
```

## 6. Set Up Reverse Proxy (Optional but Recommended)

For HTTPS support, install Nginx + Certbot:

```bash
sudo apt install -y nginx certbot python3-certbot-nginx

# Create Nginx config
sudo tee /etc/nginx/sites-available/codrix <<EOF
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_read_timeout 120s;
    }
}
EOF

sudo ln -s /etc/nginx/sites-available/codrix /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx

# SSL (if you have a domain)
sudo certbot --nginx -d your-domain.com
```

## 7. Update Frontend

After the EC2 backend is running, set the Vercel environment variable:

```
VITE_API_URL=http://<EC2_PUBLIC_IP>:8080
```

Or if using a domain with HTTPS:
```
VITE_API_URL=https://your-domain.com
```

## Useful Commands

```bash
# Restart
docker compose -f docker-compose.backend.yml restart

# Stop
docker compose -f docker-compose.backend.yml down

# Rebuild after code changes
git pull
docker compose -f docker-compose.backend.yml up -d --build

# View logs
docker compose -f docker-compose.backend.yml logs -f vyuha-backend
```
