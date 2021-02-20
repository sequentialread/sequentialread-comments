FROM node:15.9.0-alpine3.13 as build
RUN mkdir /build
WORKDIR /build
COPY package.json .
RUN npm install

FROM node:15.9.0-alpine3.13
WORKDIR /app
COPY --from=build /build/node_modules /app/node_modules
COPY . /app/
CMD [ "index.js" ]
