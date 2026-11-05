#!/usr/bin/env python3
"""Explicit demo setup: never invoked implicitly by the generic runner."""
import argparse
import json
import uuid
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen


def call(url, body=None, generation=None):
    headers = {}
    if generation is not None:
        headers['X-Demo-Generation'] = str(generation)
    payload = None
    if body is not None:
        headers['Content-Type'] = 'application/json'
        payload = json.dumps(body).encode()
    try:
        with urlopen(Request(url, data=payload, headers=headers, method='POST'), timeout=10) as response:
            return json.load(response)
    except HTTPError as error:
        raise SystemExit(f'{error.code}: {error.read().decode()}') from error


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['prepare', 'close'])
    parser.add_argument('--api', default='http://127.0.0.1:8082/demo', help='Receiver base reachable from this script')
    parser.add_argument('--target', help='Receiver base reachable from the runner; defaults to --api')
    parser.add_argument('--mode', choices=['corrected', 'faulty'], default='corrected')
    parser.add_argument('--pattern', choices=['normal', 'sequential', 'concurrent'], default='concurrent')
    parser.add_argument('--output', default='.local/scenario.json')
    parser.add_argument('--case', help='Case identifier for explicit close')
    parser.add_argument('--generation', default='1')
    parser.add_argument('--reason', choices=['COMPLETED', 'ABANDONED'], default='COMPLETED')
    args = parser.parse_args()
    api = args.api.rstrip('/') + '/v1/demo/' + args.mode + '/cases'
    if args.action == 'close':
        if not args.case:
            parser.error('--case is required for close')
        print(json.dumps(call(api + '/' + args.case + '/close', {'reason': args.reason}, args.generation), indent=2))
        return
    token = uuid.uuid4().hex
    case, parcel, event = 'case-' + token, 'parcel-' + token + '-1', 'evt-' + token + '-1'
    call(api, {'caseId': case, 'parcelIds': [parcel, 'parcel-' + token + '-2']})
    call(api + '/' + case + '/activate', generation='1')
    base = (args.target or args.api).rstrip('/') + '/v1/demo/' + args.mode + '/cases/' + case
    copies = {'normal': 1, 'sequential': 5, 'concurrent': 10}[args.pattern]
    scenario = {
        'version': 1, 'name': args.mode + ' / ' + args.pattern, 'description': 'Explicitly prepared fresh parcel case.',
        'targetUrl': base + '/events', 'headers': {'Content-Type': 'application/json', 'X-Demo-Generation': '1'},
        'body': json.dumps({'schemaVersion': '1', 'eventId': event, 'type': 'parcel.delivered', 'parcelId': parcel}),
        'copies': copies, 'concurrency': 10 if args.pattern == 'concurrent' else 1, 'timeoutMs': 5000, 'expectedStatuses': [200],
        'verification': {'url': base + '/parcels/' + parcel + '/state', 'headers': {'X-Demo-Generation': '1'},
                         'assertions': [{'pointer': '/state', 'equals': 'DELIVERED'}, {'pointer': '/notificationCount', 'equals': 1}],
                         'pollIntervalMs': 100, 'timeoutMs': 5000, 'requestTimeoutMs': 1000, 'stableForMs': 500},
    }
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(scenario, indent=2) + '\n')
    print(f'Prepared {args.mode} case {case}; generation 1. Scenario: {output}')
    print(f'After the run: python3 scripts/demo.py close --api {args.api} --mode {args.mode} --case {case}')


if __name__ == '__main__':
    main()
