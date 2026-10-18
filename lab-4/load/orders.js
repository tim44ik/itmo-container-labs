import http from 'k6/http';
import { check } from 'k6';

export const options = {
  scenarios: {
    orders: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.RATE || 5),
      timeUnit: '1s',
      duration: __ENV.DURATION || '15m',
      preAllocatedVUs: 10,
      maxVUs: 30,
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<300'],
    http_req_failed: ['rate<0.01'],
    checks: ['rate>0.99'],
    dropped_iterations: ['count==0'],
  },
};

export default function () {
  const response = http.post(
    `${__ENV.BASE_URL}/order`,
    JSON.stringify({ item: __ENV.RUN_ID, quantity: 1 }),
    {
      headers: { 'Content-Type': 'application/json' },
      tags: { name: 'POST /order' },
      timeout: '10s',
    },
  );

  check(response, {
    'заказ создан и имеет id': (r) => {
      if (r.status !== 200 && r.status !== 201) return false;
      try {
        return Number(r.json('id')) > 0;
      } catch (_) {
        return false;
      }
    },
  });
}
