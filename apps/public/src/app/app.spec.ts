import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { CpMatchHighlight } from '@cloud-presets/ui';
import { App } from './app';

describe('App', () => {
  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [App],
      providers: [provideRouter([])],
    }).compileComponents();
  });

  it('renders the brand header', async () => {
    const fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.querySelector('.app-header__brand')?.textContent).toContain('Cold Crabby');
  });

  it('consumes the shared match-highlight primitive', async () => {
    const fixture = TestBed.createComponent(CpMatchHighlight);
    fixture.componentRef.setInput('value', 'Prusa MK4');
    fixture.componentRef.setInput('ranges', [[6, 9]]);
    await fixture.whenStable();
    const mark = (fixture.nativeElement as HTMLElement).querySelector('mark');
    expect(mark?.textContent).toBe('MK4');
  });
});
