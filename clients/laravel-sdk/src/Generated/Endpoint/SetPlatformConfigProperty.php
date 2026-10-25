<?php

namespace FlowCatalyst\Generated\Endpoint;

class SetPlatformConfigProperty extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $app;
    protected $section;
    protected $property;
    /**
     * @param string $app
     * @param string $section
     * @param string $property
     * @param null|\FlowCatalyst\Generated\Model\SetPropertyRequest $requestBody
     * @param array{
     *    "clientId"?: string, //Scope to one client: the client's id (clt_...) or its identifier (e.g. acme); the platform stores the id. Omit for the application's global value. An unknown client is 404.
     * } $queryParameters
     * @param array{
     *    "X-On-Behalf-Of"?: string, //Who the calling service account acts for (for example the application's signed-in user), recorded in the platform's audit row. Honoured only from a service account; ignored from anyone else. At most 256 printable characters.
     * } $headerParameters
     */
    public function __construct(string $app, string $section, string $property, ?\FlowCatalyst\Generated\Model\SetPropertyRequest $requestBody = null, array $queryParameters = [], array $headerParameters = [])
    {
        $this->app = $app;
        $this->section = $section;
        $this->property = $property;
        $this->body = $requestBody;
        $this->queryParameters = $queryParameters;
        $this->headerParameters = $headerParameters;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'PUT';
    }
    public function getUri(): string
    {
        return str_replace(['{app}', '{section}', '{property}'], [$this->app, $this->section, $this->property], '/api/config/{app}/{section}/{property}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\SetPropertyRequest) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    protected function getQueryOptionsResolver(): \Symfony\Component\OptionsResolver\OptionsResolver
    {
        $optionsResolver = parent::getQueryOptionsResolver();
        $optionsResolver->setDefined(['clientId']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('clientId', ['string']);
        return $optionsResolver;
    }
    protected function getHeadersOptionsResolver(): \Symfony\Component\OptionsResolver\OptionsResolver
    {
        $optionsResolver = parent::getHeadersOptionsResolver();
        $optionsResolver->setDefined(['X-On-Behalf-Of']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('X-On-Behalf-Of', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null|\FlowCatalyst\Generated\Model\ConfigResponse|\FlowCatalyst\Generated\Model\ErrorModel
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ConfigResponse', 'json');
        }
        if (mb_strpos(strtolower($contentType), 'application/json') !== false) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorModel', 'json');
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}