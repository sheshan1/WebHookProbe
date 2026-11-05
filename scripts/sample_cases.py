#!/usr/bin/env python3
"""Generate UI-ready scenarios or verify live HTTP workflows. Uses Python stdlib only."""
import argparse
import copy
import json
import time
import uuid
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen
from urllib.parse import urlsplit, urlunsplit


def http(method, url, body=None, headers=None):
    headers = dict(headers or {})
    if isinstance(body, (dict, list)):
        body = json.dumps(body).encode()
        headers.setdefault('Content-Type', 'application/json')
    if isinstance(body, str):
        body = body.encode()
    request = Request(url, data=body, headers=headers, method=method)
    try:
        response = urlopen(request, timeout=15)
    except HTTPError as error:
        response = error
    with response:
        raw = response.read().decode()
        try:
            result = json.loads(raw)
        except ValueError:
            result = raw
        return response.status, result, dict(response.headers)


def scenarios(base):
    token = uuid.uuid4().hex
    template = {'version': 1, 'name': 'Sample test', 'description': 'Synthetic local sample receiver.',
                'targetUrl': base + '/webhook', 'headers': {'Content-Type': 'application/json'},
                'body': '', 'copies': 10, 'concurrency': 10, 'timeoutMs': 5000, 'expectedStatuses': [200]}
    variants = {}
    for name in ['verified', 'delivery-only', 'mismatch', 'invalid-state', 'missing-state', 'delayed',
                 'timeout', 'unreachable', 'http-error', 'redirect', 'large-response',
                 'cancel-delivery', 'cancel-verification', 'redaction']:
        s = copy.deepcopy(template)
        event_id = name + '-' + token
        s['name'] = name
        s['body'] = json.dumps({'eventId': event_id})
        if name not in ['delivery-only', 'timeout', 'unreachable', 'http-error', 'redirect',
                        'large-response', 'cancel-delivery', 'redaction']:
            path = {'mismatch': 'mismatch', 'invalid-state': 'invalid', 'missing-state': 'missing',
                    'delayed': 'delayed', 'cancel-verification': 'mismatch'}.get(name, 'state')
            s['verification'] = {'url': f'{base}/{path}/{event_id}', 'headers': {},
                                 'assertions': [{'pointer': '/notificationCount', 'equals': 1}],
                                 'pollIntervalMs': 100, 'timeoutMs': 5000, 'requestTimeoutMs': 1000,
                                 'stableForMs': 500}
        paths = {'timeout': 'slow', 'http-error': 'error', 'redirect': 'redirect',
                 'large-response': 'large', 'cancel-delivery': 'slow', 'redaction': 'echo'}
        if name in paths:
            s['targetUrl'] = base + '/' + paths[name]
        if name == 'timeout':
            s.update(copies=1, concurrency=1, timeoutMs=100)
        if name == 'unreachable':
            url = urlsplit(base)
            s.update(targetUrl=urlunsplit((url.scheme, url.hostname + ':1', '/webhook', '', '')), copies=1, concurrency=1)
        if name == 'cancel-delivery':
            s.update(copies=100, concurrency=2, timeoutMs=10000)
        if name == 'cancel-verification':
            s['verification']['timeoutMs'] = 30000
        if name == 'redaction':
            s['headers']['Authorization'] = 'Bearer sample-dummy-secret'
        variants[name] = s
    return variants


class Verifier:
    def __init__(self, args):
        self.args = args
        self.checks = []
        self.reports = {}
        self.base = args.receiver.rstrip('/') + '/v1/demo/corrected/cases'
        self.runner = args.runner.rstrip('/')
        self.context = None

    def expect(self, label, actual, expected):
        if actual != expected:
            raise AssertionError(f'{label}: expected {expected!r}, got {actual!r}')

    def check(self, label, fn):
        started = time.monotonic()
        try:
            fn()
            item = {'name': label, 'result': 'passed', 'seconds': round(time.monotonic() - started, 3)}
        except Exception as error:
            item = {'name': label, 'result': 'failed', 'detail': str(error)}
        self.checks.append(item)
        print(f"{item['result'].upper()}: {label}" + (f" — {item['detail']}" if 'detail' in item else ''), flush=True)

    def java(self, method, suffix='', body=None, status=200, generation='1', headers=None):
        context = {} if generation is None else {'X-Demo-Generation': generation}
        context.update(headers or {})
        result = http(method, self.context + suffix, body, context)
        self.expect(f'{method} {suffix} status', result[0], status)
        if status >= 400:
            self.expect('problem JSON status', result[1]['status'], status)
            self.expect('request ID echo', bool(result[2].get('X-Request-Id')), True)
        return result[1]

    def fresh(self):
        case = 'audit-' + uuid.uuid4().hex
        response = http('POST', self.base, {'caseId': case, 'parcelIds': ['p1', 'p2']})
        self.expect('create', response[0], 201)
        self.context = self.base + '/' + case
        self.java('POST', '/activate')

    def event(self, event='e1', parcel='p1'):
        return {'schemaVersion': '1', 'eventId': event, 'type': 'parcel.delivered', 'parcelId': parcel}

    def finish(self, run_id):
        deadline = time.monotonic() + 40
        while time.monotonic() < deadline:
            status, report, _ = http('GET', self.runner + '/v1/runs/' + run_id)
            self.expect('poll', status, 200)
            if report['finishedAt']:
                self.reports[report['scenario']['name']] = report
                return report
            time.sleep(.05)
        raise AssertionError('run did not terminate')

    def start(self, scenario):
        status, report, _ = http('POST', self.runner + '/v1/runs', scenario)
        self.expect('run accepted', status, 202)
        return report['id']

    def scenario(self, scenario, verdict, business):
        report = self.finish(self.start(scenario))
        self.expect('verdict', report['verdict'], verdict)
        self.expect('business outcome', report['business']['outcome'], business)
        self.expect('exact attempt count', len(report['attempts']), scenario['copies'])
        return report

    def receiver_checks(self):
        self.fresh()
        def normal():
            self.expect('first outcome', self.java('POST', '/events', self.event())['outcome'], 'PROCESSED')
            self.expect('notification count', self.java('GET', '/parcels/p1/state')['notificationCount'], 1)
        self.check('normal event and parcel inspection', normal)
        self.check('semantic duplicate', lambda: self.expect('duplicate', self.java('POST', '/events', dict(reversed(list(self.event().items()))))['outcome'], 'DUPLICATE'))
        self.check('distinct event same parcel', lambda: self.expect('already delivered', self.java('POST', '/events', self.event('e2'))['outcome'], 'ALREADY_DELIVERED'))
        self.check('event identity conflict precedes unknown parcel', lambda: self.java('POST', '/events', self.event('e1', 'missing'), status=409))
        self.check('new event with unknown parcel', lambda: self.java('POST', '/events', self.event('unknown', 'missing'), status=404))
        for label, body, status, headers in [
            ('malformed JSON', '{', 400, {'Content-Type': 'application/json'}),
            ('duplicate JSON keys', '{"eventId":"a","eventId":"b"}', 400, {'Content-Type': 'application/json'}),
            ('unknown field', dict(self.event(), extra=True), 400, {}),
            ('missing fields', {}, 422, {}),
            ('wrong field type', dict(self.event(), schemaVersion=1), 422, {}),
            ('unsupported type', dict(self.event(), type='parcel.other'), 422, {}),
            ('unsupported version', dict(self.event(), schemaVersion='2'), 422, {}),
            ('invalid identifier', dict(self.event(), eventId='bad id'), 422, {}),
            ('wrong media type', json.dumps(self.event()), 415, {'Content-Type': 'text/plain'}),
            ('oversized body', ' ' * 16385, 413, {'Content-Type': 'application/json'}),
        ]:
            self.check(label, lambda b=body, st=status, h=headers: self.java('POST', '/events', b, st, headers=h))
        self.check('missing generation', lambda: self.java('GET', '/state', status=400, generation=None))
        self.check('wrong generation', lambda: self.java('GET', '/state', status=409, generation='2'))
        self.check('unsupported method', lambda: self.java('PUT', '/events', status=405))
        self.check('unsupported Accept', lambda: self.java('GET', '/state', status=406, headers={'Accept': 'text/html'}))
        self.check('correlation ID and no-store', lambda: self.expect('cache-control', http('GET', self.context, headers={'X-Request-Id': 'audit-request'})[2].get('Cache-Control'), 'no-store'))
        def invariant():
            state = self.java('GET', '/parcels/p1/state')
            self.expect('count unchanged by errors', state['notificationCount'], 1)
            self.expect('accepted distinct events', state['acceptedEventCount'], 2)
        self.check('invalid inputs leave business state unchanged', invariant)
        self.check('ACTIVE reset rejected', lambda: self.java('POST', '/reset', status=409))
        def reset():
            self.java('POST', '/close', {'reason': 'COMPLETED'})
            result = self.java('POST', '/reset')
            self.expect('new generation', result['case']['generation'], '2')
            self.java('GET', '/state', status=409)
            self.java('POST', '/activate', generation='2')
            self.java('POST', '/events', self.event(), status=409)
            state = self.java('GET', '/parcels/p1/state', generation='2')
            self.expect('reset state', state['state'], 'IN_TRANSIT')
            self.expect('reset count', state['notificationCount'], 0)
            self.java('POST', '/close', {'reason': 'ABANDONED'}, generation='2')
        self.check('close/reset/reactivate rejects stale context and clears state', reset)
        self.check('case discovery after reset without generation header', lambda: self.expect('discovered generation', self.java('GET', generation=None)['generation'], '2'))
        def metadata_isolation():
            self.fresh()
            self.expect('new case clean', self.java('GET', '/parcels/p1/state')['notificationCount'], 0)
            self.java('POST', '/close', {'reason': 'ABANDONED'})
        self.check('case isolation', metadata_isolation)

    def runner_checks(self):
        variants = scenarios(self.args.fixture)
        for name, verdict, business in [
            ('verified', 'passed', 'passed'), ('delivery-only', 'inconclusive', 'not_configured'),
            ('mismatch', 'failed', 'failed'), ('invalid-state', 'inconclusive', 'inconclusive'),
            ('missing-state', 'inconclusive', 'inconclusive'), ('delayed', 'passed', 'passed'),
            ('timeout', 'failed', 'not_configured'), ('unreachable', 'failed', 'not_configured'),
            ('http-error', 'failed', 'not_configured'), ('redirect', 'failed', 'not_configured'),
            ('large-response', 'inconclusive', 'not_configured'), ('redaction', 'inconclusive', 'not_configured'),
        ]:
            def execute(n=name, v=verdict, b=business):
                report = self.scenario(variants[n], v, b)
                if n == 'timeout':
                    self.expect('timeout classification', report['attempts'][0]['outcome'], 'timeout')
                if n == 'unreachable':
                    self.expect('transport classification', report['attempts'][0]['outcome'], 'transport_error')
                if n == 'redirect':
                    self.expect('redirect not followed', report['attempts'][0]['status'], 307)
                if n == 'large-response':
                    self.expect('truncated', report['attempts'][0]['truncated'], True)
                    self.expect('preview bytes', len(report['attempts'][0]['preview']), 4096)
                if n == 'redaction':
                    self.expect('no dummy secret leaked', 'sample-dummy-secret' in json.dumps(report), False)
                    self.expect('redaction marker', '[REDACTED]' in json.dumps(report), True)
            self.check('runner: ' + name, execute)
        def delivery_cancel():
            run_id = self.start(variants['cancel-delivery'])
            status, _, _ = http('POST', self.runner + '/v1/runs', variants['verified'])
            self.expect('one active run', status, 409)
            status, partial, _ = http('GET', self.runner + '/v1/runs/' + run_id + '/report')
            self.expect('partial report available', status, 200)
            self.expect('partial report unfinished', partial['finishedAt'], None)
            time.sleep(.2)
            http('POST', self.runner + '/v1/runs/' + run_id + '/cancel')
            report = self.finish(run_id)
            self.expect('cancelled', report['state'], 'cancelled')
            self.expect('inconclusive', report['verdict'], 'inconclusive')
            self.expect('unstarted requests remain', report['unstarted'] > 0, True)
        self.check('delivery cancellation, partial report, single-active admission', delivery_cancel)
        def verification_cancel():
            run_id = self.start(variants['cancel-verification'])
            deadline = time.monotonic() + 5
            while time.monotonic() < deadline:
                _, report, _ = http('GET', self.runner + '/v1/runs/' + run_id)
                if report['state'] == 'verifying':
                    break
                time.sleep(.05)
            self.expect('entered verifying', report['state'], 'verifying')
            http('POST', self.runner + '/v1/runs/' + run_id + '/cancel')
            report = self.finish(run_id)
            self.expect('cancelled', report['state'], 'cancelled')
            self.expect('business inconclusive', report['business']['outcome'], 'inconclusive')
        self.check('verification cancellation', verification_cancel)
        self.check('invalid scenario rejected', lambda: self.expect('validation status', http('POST', self.runner + '/v1/runs', {})[0], 422))
        self.check('runner media type rejected', lambda: self.expect('media status', http('POST', self.runner + '/v1/runs', '{}', {'Content-Type': 'text/plain'})[0], 415))
        self.check('unknown run unavailable', lambda: self.expect('lookup', http('GET', self.runner + '/v1/runs/missing')[0], 404))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['generate', 'verify'])
    parser.add_argument('--fixture', default='http://fixture:8090', help='Fixture address as seen by runner')
    parser.add_argument('--runner', default='http://127.0.0.1:8082/api')
    parser.add_argument('--receiver', default='http://127.0.0.1:8082/demo')
    parser.add_argument('--output', default='.local/samples')
    args = parser.parse_args()
    output = Path(args.output)
    output.mkdir(parents=True, exist_ok=True)
    if args.action == 'generate':
        for name, scenario in scenarios(args.fixture).items():
            (output / (name + '.json')).write_text(json.dumps(scenario, indent=2) + '\n')
        print(f'Generated 14 scenarios in {output}. Import individually; cancellation cases need your Cancel action.')
        return 0
    verifier = Verifier(args)
    verifier.receiver_checks()
    verifier.runner_checks()
    for name, report in verifier.reports.items():
        (output / (name + '-report.json')).write_text(json.dumps(report, indent=2) + '\n')
    failed = sum(c['result'] == 'failed' for c in verifier.checks)
    (output / 'results.json').write_text(json.dumps({'checks': verifier.checks, 'failed': failed}, indent=2) + '\n')
    print(f'{len(verifier.checks)-failed}/{len(verifier.checks)} workflow checks passed. Evidence: {output}')
    return int(failed > 0)


if __name__ == '__main__':
    raise SystemExit(main())
