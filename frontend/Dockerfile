FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json* bun.lock* ./
RUN npm install --legacy-peer-deps
COPY . .
RUN npm run build

# The dist/ folder is the output — copied to a shared volume by docker-compose
FROM alpine:3.21
COPY --from=build /app/dist /app/dist
CMD ["echo", "Frontend build complete"]
