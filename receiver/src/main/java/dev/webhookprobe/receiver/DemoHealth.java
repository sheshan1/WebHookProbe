package dev.webhookprobe.receiver;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.health.contributor.Health;
import org.springframework.boot.health.contributor.HealthIndicator;
import org.springframework.stereotype.Component;

@Component("demo")
class DemoHealth implements HealthIndicator {

    private final boolean enabled;

    DemoHealth(@Value("${webhookprobe.demo.enabled:false}") boolean enabled) {
        this.enabled = enabled;
    }

    @Override
    public Health health() {
        return enabled ? Health.up().build() : Health.outOfService().build();
    }

}
