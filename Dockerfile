FROM ubuntu:18.04 as base
ENV PORTER_HOME="/root/.porter"
ENV	PORTER_VERSION=latest
ENV CNAB_AZURE_HOME="/root/.cnab-azure-driver"
RUN apt update; \
    apt install curl -y; \ 
    apt install jq -y; \
    mkdir -p ${PORTER_HOME}; \ 
    curl -fsSLo ${PORTER_HOME}/porter https://cdn.porter.sh/${PORTER_VERSION}/porter-linux-amd64; \
    chmod +x ${PORTER_HOME}/porter; \
    ${PORTER_HOME}/porter plugin install azure --version $PORTER_VERSION; \
    DOWNLOAD_LOCATION=$( curl -sL https://api.github.com/repos/deislabs/cnab-azure-driver/releases/latest | jq '.assets[]|select(.name=="cnab-azure-linux-amd64").browser_download_url' -r) ; \
		mkdir -p ${CNAB_AZURE_HOME}; \
		curl -sSLo  ${CNAB_AZURE_HOME}/cnab-azure ${DOWNLOAD_LOCATION}; \
		chmod +x ${CNAB_AZURE_HOME}/cnab-azure;
COPY /setup/config.toml ${PORTER_HOME}
ENV PATH=${PORTER_HOME}:${CNAB_AZURE_HOME}:${PATH}
COPY /bin/cnabcustomrphandler /
WORKDIR /
ENTRYPOINT ["/cnabcustomrphandler"]