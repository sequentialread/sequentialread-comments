FROM node:4.6.0-rpi

RUN mkdir -p /usr/src/app
WORKDIR /usr/src/app

# Install app dependencies
COPY package.json /usr/src/app/
RUN npm install

RUN npm install leveldown --build-from-source

# Bundle app source
COPY . /usr/src/app

EXPOSE 2369

CMD [ "node", "node_modules/nodemon/bin/nodemon.js", "index" ]
