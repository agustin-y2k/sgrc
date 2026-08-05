# Build estático — mismo criterio multi-stage que el Dockerfile de sgrc-app
# (ver ../Dockerfile): la imagen final no lleva Node ni node_modules, solo
# los assets ya compilados servidos por nginx.
FROM node:24-alpine AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
COPY --from=build /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
