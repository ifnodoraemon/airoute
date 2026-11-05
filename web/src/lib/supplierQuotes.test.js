import { describe, expect, it } from 'vitest';
import { compareQuote, procurementRows } from './supplierQuotes';

const prices = [
  { model: 'public-a', group_name: 'vip', prompt_price: 99, completion_price: 99 },
  {
    model: 'public-a',
    group_name: 'default',
    prompt_price: 4,
    completion_price: 8,
    currency: 'USD',
  },
];
const quote = {
  channel: 'proxy',
  model: 'physical',
  currency: 'USD',
  billing_mode: 'token',
  input_cost: 1,
  output_cost: 2,
  status: 'active',
};
describe('procurement identity and comparable rates', () => {
  it('joins quotes by actual provider model and shares a quote across public aliases', () => {
    const rows = procurementRows(
      [{ id: 1, name: 'proxy', upstream_models: ['physical'] }],
      [
        { model: 'public-a', providers: [{ channel_id: 1, mapped_model: 'physical' }] },
        { model: 'public-b', providers: [{ channel_id: 1, mapped_model: 'physical' }] },
        { model: 'unquoted', providers: [{ channel_id: 1, mapped_model: 'other' }] },
      ],
      [quote]
    );
    expect(rows).toHaveLength(2);
    expect(rows[0].routes.map((row) => row.model)).toEqual(['public-a', 'public-b']);
    expect(rows[0].quote).toBe(quote);
    expect(rows[1].quote).toBeUndefined();
    expect(compareQuote(rows[0].quote, prices, 'public-a')).toMatchObject({ input: 3, output: 6 });
  });
  it('keeps orphan quotes without pretending that their names are public routes', () => {
    const rows = procurementRows([], [], [quote]);
    expect(rows[0].routes).toEqual([]);
    expect(rows[0].quote).toBe(quote);
  });
  it('does not substitute a VIP tier, convert currencies, compare disabled or missing quotes', () => {
    expect(compareQuote(quote, prices.slice(0, 1), 'public-a').reason).toBe('unpriced');
    expect(compareQuote({ ...quote, currency: 'CNY' }, prices, 'public-a').reason).toBe('currency');
    expect(compareQuote({ ...quote, status: 'disabled' }, prices, 'public-a').reason).toBe(
      'disabled'
    );
    expect(compareQuote(null, prices, 'public-a').reason).toBe('missing');
  });
  it('compares per-request prices only against a flat retail request price', () => {
    const request = { ...quote, billing_mode: 'request', fixed_cost: 0.05 };
    expect(compareQuote(request, prices, 'public-a').reason).toBe('basis');
    expect(
      compareQuote(
        request,
        [
          {
            model: 'public-a',
            currency: 'USD',
            prompt_price: 0,
            completion_price: 0,
            fixed_price: 0.1,
          },
        ],
        'public-a'
      ).fixed
    ).toBeCloseTo(0.05);
    expect(compareQuote(quote, [{ ...prices[1], fixed_price: 0.1 }], 'public-a').reason).toBe(
      'basis'
    );
  });
});

it('uses the server billing resolver for wildcard prices and labels system fallback as unconfigured', () => {
  const quote = {currency:'USD', billing_mode:'token', input_cost:1,output_cost:2,status:'active'};
  const reference = {retail_price:{model:'gemini-*',currency:'USD',prompt_price:4,completion_price:8},retail_price_source:'wildcard'};
  expect(compareQuote(quote, [], 'gemini-public', reference)).toMatchObject({input:3,output:6});
  expect(compareQuote(quote, [], 'unpriced', {...reference,retail_price_source:'default'}).reason).toBe('unpriced');
  const request = {...quote,billing_mode:'request',fixed_cost:0.1};
  expect(compareQuote(request, [{model:'images',currency:'USD',prompt_price:0,completion_price:0,fixed_price:0.2}], 'images', {modality:'image'}).reason).toBe('basis');
});
