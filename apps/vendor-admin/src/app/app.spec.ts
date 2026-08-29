import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { SessionStore } from './auth/session';
import { App } from './app';

describe('App', () => {
  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [App],
      providers: [provideRouter([])],
    }).compileComponents();
  });

  it('renders the vendor admin header', async () => {
    const fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.querySelector('.app-header__title')?.textContent).toContain('Vendor Admin');
  });
});

describe('SessionStore', () => {
  it('exposes the session as signals and toggles on sign-in/out', () => {
    const store = TestBed.inject(SessionStore);
    expect(store.isAuthenticated()).toBe(false);
    store.signIn();
    expect(store.isAuthenticated()).toBe(true);
    expect(store.member()?.organizationName).toBe('Prusa Research');
    store.signOut();
    expect(store.isAuthenticated()).toBe(false);
  });
});
