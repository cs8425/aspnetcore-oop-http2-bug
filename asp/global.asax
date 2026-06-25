<%@ Application Language="C#" %>
<%@ Import Namespace="System" %>
<%@ Import Namespace="System.IO" %>
<%@ Import Namespace="System.Web" %>

<script runat="server">
	void Application_Start(object sender, EventArgs e) {
		// runs on application startup
	}

	void Application_End(object sender, EventArgs e) {
		// runs on application shutdown
	}

	void Application_Error(object sender, EventArgs e) {
		// runs when an unhandled error occurs
	}

	void Application_BeginRequest(object sender, EventArgs e)
	{
		// Cast the sender to HttpApplication to get the current context
		HttpApplication app = (HttpApplication)sender;
		HttpContext context = app.Context;

		// context.Response.Headers.Add("X-Custom-Global-Header", "ProcessedByGlobalAsax");

		context.Response.ContentType = "text/plain; charset=utf-8";
		context.Response.Write(string.Format("SERVER_PROTOCOL={0}\n", context.Request.ServerVariables["SERVER_PROTOCOL"]));
		context.Response.Write(string.Format("REQUEST_METHOD={0}\n", context.Request.ServerVariables["REQUEST_METHOD"]));
		context.Response.Write(string.Format("REMOTE_ADDR={0}\n", context.Request.ServerVariables["REMOTE_ADDR"]));
		context.Response.Write("=================================\n");
		context.Response.Write(string.Format("context.Request.HttpMethod={0}\n",  context.Request.HttpMethod));
		context.Response.Write(string.Format("context.Request.Url={0}\n",  context.Request.Url.ToString()));
		context.Response.Write(string.Format("context.Request.ApplicationPath={0}\n", context.Request.ApplicationPath));
		context.Response.Write(string.Format("context.Request.CurrentExecutionFilePath={0}\n", context.Request.CurrentExecutionFilePath));
		// context.Response.Write(string.Format("context.Request.context.Request.PhysicalApplicationPath={0}\n", context.Request.PhysicalApplicationPath));
		context.Response.Write("=================================\n");
		foreach (var headerKey in context.Request.Headers.AllKeys)
		{
			string headerValue = context.Request.Headers[headerKey];
			context.Response.Write(headerKey + ": " + headerValue + "\n");
		}
		context.Response.Write("=================================\n");

		// stop other handler
		app.CompleteRequest();
	}

	void Session_Start(object sender, EventArgs e) {
		// runs when a new session is started
	}

	void Session_End(object sender, EventArgs e) {
		// runs when a session ends
	}
</script>