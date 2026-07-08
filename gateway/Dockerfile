# Stage 1: Build Golang Daemons
FROM golang:alpine AS daemon-builder
WORKDIR /build

# Copy go module files
COPY daemon/go.mod daemon/go.sum ./
RUN go mod download

# Copy rest of daemon source
COPY daemon/ ./
RUN go mod tidy

# Compile for both Linux and Windows VPS targets
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o promptops-daemon-linux
RUN CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o promptops-daemon-windows.exe

# Stage 2: Build Gateway
FROM node:20-alpine AS gateway-builder
WORKDIR /app
COPY gateway/package*.json ./
RUN npm ci
COPY gateway/ ./
RUN npm run build

# Stage 3: Runner
FROM node:20-alpine
WORKDIR /app
COPY gateway/package*.json ./
COPY gateway/drizzle.config.ts ./
# Install all dependencies (including devDependencies so drizzle-kit has typescript support to parse schema)
RUN npm ci

COPY gateway/src ./src
COPY --from=gateway-builder /app/dist ./dist

# Copy the compiled daemon binaries so the gateway can serve them
COPY --from=daemon-builder /build/promptops-daemon-linux /daemon/promptops-daemon-linux
COPY --from=daemon-builder /build/promptops-daemon-windows.exe /daemon/promptops-daemon-windows.exe

# Copy the shared proto files
COPY shared /shared

EXPOSE 3001
ENV PORT=3001
ENV HOST=0.0.0.0
ENV NODE_ENV=production

CMD ["node", "dist/index.js"]
