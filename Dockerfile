FROM node:14-buster

RUN mkdir -p /usr/src/app
WORKDIR /usr/src/app

# Install app dependencies
COPY package.json /usr/src/app/
RUN npm install

RUN npm install leveldown --build-from-source

# Bundle app source
COPY . /usr/src/app

EXPOSE 2369

CMD [ "bash", "restart-on-crash.sh" ]
