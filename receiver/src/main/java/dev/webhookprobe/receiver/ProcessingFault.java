package dev.webhookprobe.receiver;

import org.springframework.stereotype.Component;

/** Overridden only by an in-process test-profile fixture; no HTTP fault switch. */
@Component
public class ProcessingFault {

    public void beforeNotification() {
    }

}
