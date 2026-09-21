# TODO(ramishra): remove fallback once antelope images are no longer tested
try:
    from heat.wsgi.cfn import application
except ImportError:
    from heat.httpd.heat_api_cfn import init_application

    application = init_application()
