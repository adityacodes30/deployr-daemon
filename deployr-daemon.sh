set -e  
set -o pipefail  

if [ -z "$1" ]; then
  echo "Error: No Next.js Git repository URL provided."
  echo "Usage: $0 <nextjs_repo_url>"
  exit 1
fi

# Variables
REPO_URL="$1"
NEXTJS_DIR="$HOME/nextjs-app"

if [ ! -d "$NEXTJS_DIR" ]; then
  echo "Directory not found. Cloning the repository..."
  git clone "$REPO_URL" "$NEXTJS_DIR"
else
  echo "Repository already exists. Pulling the latest changes..."
  cd "$NEXTJS_DIR"
  git reset --hard
  git pull origin main
fi

cd "$NEXTJS_DIR"

if [ -d ".next" ]; then
  echo "Removing previous build..."
  rm -rf .next
fi

echo "Installing dependencies (this might take a few minutes)..."
npm install

echo "Building the Next.js app..."
npm run build

echo "Restarting Next.js app with PM2..."
pm2 delete nextjs-app || true  
pm2 start "npm run start" --name nextjs-app --cwd "$NEXTJS_DIR"

pm2 save

echo "Deployment completed successfully!"
