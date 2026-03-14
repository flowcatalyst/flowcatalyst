<?php

namespace FlowCatalyst\Generated\Endpoint;

class PutApiAdminConfigByAppCodeBySectionByProperty extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $appCode;
    protected $section;
    protected $property;
    /**
     * @param string $appCode
     * @param string $section
     * @param string $property
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutBody $requestBody
     * @param array{
     *    "scope"?: string,
     *    "clientId"?: string,
     * } $queryParameters
     */
    public function __construct(string $appCode, string $section, string $property, ?\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutBody $requestBody = null, array $queryParameters = [])
    {
        $this->appCode = $appCode;
        $this->section = $section;
        $this->property = $property;
        $this->body = $requestBody;
        $this->queryParameters = $queryParameters;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'PUT';
    }
    public function getUri(): string
    {
        return str_replace(['{appCode}', '{section}', '{property}'], [$this->appCode, $this->section, $this->property], '/api/admin/config/{appCode}/{section}/{property}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutBody) {
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
        $optionsResolver->setDefined(['scope', 'clientId']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('scope', ['string']);
        $optionsResolver->addAllowedTypes('clientId', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminConfigByAppCodeBySectionByPropertyBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminConfigByAppCodeBySectionByPropertyForbiddenException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse200|\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse201
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse200', 'json');
        }
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse201', 'json');
        }
        if (is_null($contentType) === false && (400 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PutApiAdminConfigByAppCodeBySectionByPropertyBadRequestException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse400', 'json'), $response);
        }
        if (is_null($contentType) === false && (403 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PutApiAdminConfigByAppCodeBySectionByPropertyForbiddenException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse403', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}