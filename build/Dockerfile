FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/heat-operator \
    USER_UID=1001 \
    USER_NAME=heat-operator \
    OPERATOR_TEMPLATES=/usr/share/heat-operator/templates/

# install operator binary
COPY build/_output/bin/heat-operator ${OPERATOR}

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

# install our templates
RUN  mkdir -p ${OPERATOR_TEMPLATES}
COPY templates ${OPERATOR_TEMPLATES}

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
