FROM node:22-slim
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY tsconfig.json ./
COPY src/ ./src/
RUN rm -rf src/__tests__ && npx tsc --skipLibCheck
EXPOSE 3100
CMD ["node", "dist/index.js"]
