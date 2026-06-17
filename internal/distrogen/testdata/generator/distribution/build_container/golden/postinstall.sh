if command -v systemctl >/dev/null 2>&1; then
    if [ -d /run/systemd/system ]; then
        systemctl daemon-reload
    fi
    systemctl enable otelcol-basic.service
    if [ -f /etc/otelcol-basic/config.yaml ]; then
        if [ -d /run/systemd/system ]; then
            systemctl restart otelcol-basic.service
        fi
    fi
fi
