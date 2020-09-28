# To enable ssh & remote debugging on app service change the base image to the one below
FROM ubuntu:18.04
COPY /bin/customrphandler /
WORKDIR /
ENTRYPOINT ["/customrphandler"]